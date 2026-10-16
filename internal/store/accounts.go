package store

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

// Account errors.
var (
	ErrEmailTaken     = errors.New("store: email already registered")
	ErrBadSignupCode  = errors.New("store: invalid or used signup code")
	ErrInvalidToken   = errors.New("store: invalid or expired token")
	ErrNoUser         = errors.New("store: no such user")
	ErrSessionExpired = errors.New("store: no live session")
)

// Email token purposes.
const (
	PurposeVerifyEmail   = "verify_email"
	PurposeResetPassword = "reset_password"
)

// Email is a rendered email for the outbox: the mailer sends it as is.
type Email struct {
	To, Template, Lang, Subject, Body string
}

func enqueue(ctx context.Context, q *Queries, e Email) error {
	return q.EnqueueEmail(ctx, EnqueueEmailParams{ToEmail: e.To, Template: e.Template, Lang: e.Lang, Subject: e.Subject, BodyText: e.Body})
}

// Registration is everything a signup writes, in one transaction.
type Registration struct {
	Email, PasswordHash, Lang string
	Name                      *string
	SignupCodeHash            []byte
	OrgName                   string
	VerifyTokenHash           []byte
	VerifyTTL                 time.Duration
	VerifyEmail               Email // with the verification link
}

// Register creates the user, their organization (as owner), spends the
// signup code, stores the verification token and queues its email. A taken
// email is ErrEmailTaken and the code is not spent.
func (s *Store) Register(ctx context.Context, r Registration) (u User, org Organization, err error) {
	err = s.Tx(ctx, Public, func(q *Queries) error {
		codeID, err := q.CheckSignupCode(ctx, r.SignupCodeHash)
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrBadSignupCode
		}
		if err != nil {
			return err
		}
		u, err = q.CreateUser(ctx, CreateUserParams{Email: r.Email, PasswordHash: r.PasswordHash, Name: r.Name, Lang: r.Lang})
		var pg *pgconn.PgError
		if errors.As(err, &pg) && pg.Code == "23505" {
			return ErrEmailTaken
		}
		if err != nil {
			return err
		}
		if n, err := q.ConsumeSignupCode(ctx, ConsumeSignupCodeParams{ID: codeID, UsedBy: &u.ID}); err != nil || n != 1 {
			if err == nil {
				err = ErrBadSignupCode // spent concurrently
			}
			return err
		}
		// The new organization is the tenant from here on: RLS lets the
		// insert return it and the owner membership be written.
		orgID := uuid.New()
		if _, err := q.db.Exec(ctx, "SELECT set_config('app.org_id', $1, true), set_config('app.user_id', $2, true)",
			orgID.String(), u.ID.String()); err != nil {
			return err
		}
		if org, err = q.CreateOrganization(ctx, CreateOrganizationParams{ID: orgID, Name: r.OrgName}); err != nil {
			return err
		}
		if err := q.AddMembership(ctx, AddMembershipParams{OrganizationID: org.ID, UserID: u.ID, Role: "owner"}); err != nil {
			return err
		}
		if err := q.CreateAuthToken(ctx, CreateAuthTokenParams{UserID: u.ID, Purpose: PurposeVerifyEmail,
			TokenHash: r.VerifyTokenHash, ExpiresAt: time.Now().Add(r.VerifyTTL)}); err != nil {
			return err
		}
		return enqueue(ctx, q, r.VerifyEmail)
	})
	return u, org, err
}

// UserByEmail returns the user with that (lowercased) email, or ErrNoUser.
func (s *Store) UserByEmail(ctx context.Context, email string) (u User, err error) {
	err = s.Tx(ctx, Public, func(q *Queries) error {
		u, err = q.GetUserByEmail(ctx, email)
		return err
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return User{}, ErrNoUser
	}
	return u, err
}

// EnqueueEmail queues one rendered email.
func (s *Store) EnqueueEmail(ctx context.Context, e Email) error {
	return s.Tx(ctx, Public, func(q *Queries) error { return enqueue(ctx, q, e) })
}

// SendToken replaces the user's pending tokens of purpose with a new one and
// queues its email (resend verification, password reset).
func (s *Store) SendToken(ctx context.Context, userID uuid.UUID, purpose string, hash []byte, ttl time.Duration, e Email) error {
	return s.Tx(ctx, Tenant{UserID: &userID}, func(q *Queries) error {
		if err := q.InvalidateAuthTokens(ctx, InvalidateAuthTokensParams{UserID: userID, Purpose: purpose}); err != nil {
			return err
		}
		if err := q.CreateAuthToken(ctx, CreateAuthTokenParams{UserID: userID, Purpose: purpose, TokenHash: hash,
			ExpiresAt: time.Now().Add(ttl)}); err != nil {
			return err
		}
		return enqueue(ctx, q, e)
	})
}

// VerifyEmail spends a verification token and marks the email verified.
func (s *Store) VerifyEmail(ctx context.Context, hash []byte) (u User, err error) {
	err = s.Tx(ctx, Public, func(q *Queries) error {
		id, err := q.ConsumeAuthToken(ctx, ConsumeAuthTokenParams{TokenHash: hash, Purpose: PurposeVerifyEmail})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidToken
		}
		if err != nil {
			return err
		}
		if err := q.MarkEmailVerified(ctx, id); err != nil {
			return err
		}
		u, err = q.GetUserByID(ctx, id)
		return err
	})
	return u, err
}

// ResetPassword spends a reset token and calls prepare with its user, which
// checks the new password against the account and returns its hash and the
// "password changed" email. If prepare fails nothing changes: the token is
// still usable. Otherwise the hash is stored, every session of the user ends
// and the email is queued. Resetting also proves the email address, so it is
// marked verified.
func (s *Store) ResetPassword(ctx context.Context, hash []byte, prepare func(User) (newHash string, notify Email, err error)) (u User, err error) {
	err = s.Tx(ctx, Public, func(q *Queries) error {
		id, err := q.ConsumeAuthToken(ctx, ConsumeAuthTokenParams{TokenHash: hash, Purpose: PurposeResetPassword})
		if errors.Is(err, pgx.ErrNoRows) {
			return ErrInvalidToken
		}
		if err != nil {
			return err
		}
		if u, err = q.GetUserByID(ctx, id); err != nil {
			return err
		}
		newHash, notify, err := prepare(u)
		if err != nil {
			return err
		}
		if err := q.UpdateUserPassword(ctx, UpdateUserPasswordParams{ID: id, PasswordHash: newHash}); err != nil {
			return err
		}
		if err := q.MarkEmailVerified(ctx, id); err != nil {
			return err
		}
		if err := q.DeleteUserSessions(ctx, DeleteUserSessionsParams{UserID: id}); err != nil {
			return err
		}
		if err := q.InvalidateAuthTokens(ctx, InvalidateAuthTokensParams{UserID: id, Purpose: PurposeResetPassword}); err != nil {
			return err
		}
		return enqueue(ctx, q, notify)
	})
	return u, err
}

// ChangePassword sets a new password hash, ends the user's other sessions,
// revokes pending reset links and queues the notification email.
func (s *Store) ChangePassword(ctx context.Context, userID, keepSession uuid.UUID, newHash string, e Email) error {
	return s.Tx(ctx, Tenant{UserID: &userID}, func(q *Queries) error {
		if err := q.UpdateUserPassword(ctx, UpdateUserPasswordParams{ID: userID, PasswordHash: newHash}); err != nil {
			return err
		}
		if err := q.DeleteUserSessions(ctx, DeleteUserSessionsParams{UserID: userID, Keep: &keepSession}); err != nil {
			return err
		}
		if err := q.InvalidateAuthTokens(ctx, InvalidateAuthTokensParams{UserID: userID, Purpose: PurposeResetPassword}); err != nil {
			return err
		}
		return enqueue(ctx, q, e)
	})
}

// UpdatePasswordHash stores a rehashed password (new argon2 parameters).
func (s *Store) UpdatePasswordHash(ctx context.Context, userID uuid.UUID, newHash string) error {
	return s.Tx(ctx, Tenant{UserID: &userID}, func(q *Queries) error {
		return q.UpdateUserPassword(ctx, UpdateUserPasswordParams{ID: userID, PasswordHash: newHash})
	})
}

// StartSession records a login: a new session (token hash, CSRF token,
// first organization of the user as active) and the login time.
func (s *Store) StartSession(ctx context.Context, userID uuid.UUID, tokenHash []byte, csrf string, ttl time.Duration) (sess Session, err error) {
	err = s.Tx(ctx, Tenant{UserID: &userID}, func(q *Queries) error {
		orgs, err := q.ListUserOrganizations(ctx, userID)
		if err != nil {
			return err
		}
		var active *uuid.UUID
		if len(orgs) > 0 {
			active = &orgs[0].ID
		}
		if sess, err = q.CreateSession(ctx, CreateSessionParams{UserID: userID, TokenHash: tokenHash, CsrfToken: csrf,
			ActiveOrgID: active, ExpiresAt: time.Now().Add(ttl)}); err != nil {
			return err
		}
		return q.TouchUserLogin(ctx, userID)
	})
	return sess, err
}

// SessionByToken returns a live session, refreshing its last use if older
// than touchAfter. Expired or idle sessions are ErrSessionExpired.
func (s *Store) SessionByToken(ctx context.Context, hash []byte, idle, touchAfter time.Duration) (row GetSessionByTokenRow, err error) {
	err = s.Tx(ctx, Public, func(q *Queries) error {
		row, err = q.GetSessionByToken(ctx, GetSessionByTokenParams{TokenHash: hash, IdleSeconds: idle.Seconds()})
		if err != nil {
			return err
		}
		if time.Since(row.LastSeenAt) > touchAfter {
			return q.TouchSession(ctx, row.ID)
		}
		return nil
	})
	if errors.Is(err, pgx.ErrNoRows) {
		return GetSessionByTokenRow{}, ErrSessionExpired
	}
	return row, err
}

// EndSession deletes one session of the user (logout).
func (s *Store) EndSession(ctx context.Context, userID, sessionID uuid.UUID) error {
	return s.Tx(ctx, Tenant{UserID: &userID}, func(q *Queries) error {
		return q.DeleteSession(ctx, DeleteSessionParams{ID: sessionID, UserID: userID})
	})
}

// UserOrganizations lists the user's organizations with their role.
func (s *Store) UserOrganizations(ctx context.Context, userID uuid.UUID) (orgs []ListUserOrganizationsRow, err error) {
	err = s.Tx(ctx, Tenant{UserID: &userID}, func(q *Queries) error {
		orgs, err = q.ListUserOrganizations(ctx, userID)
		return err
	})
	return orgs, err
}

// Audit records a sensitive action. orgID is nil for account events.
func (s *Store) Audit(ctx context.Context, orgID, userID *uuid.UUID, action string, target, ipPrefix *string) error {
	return s.Tx(ctx, Tenant{OrgID: orgID, UserID: userID}, func(q *Queries) error {
		return q.InsertAudit(ctx, InsertAuditParams{OrganizationID: orgID, UserID: userID, Action: action, Target: target, IpPrefix: ipPrefix})
	})
}

// CreateSignupCode stores a beta signup code (hash only). Run as the owner
// role by cmd/qradmin.
func (s *Store) CreateSignupCode(ctx context.Context, hash []byte, note *string, ttl time.Duration) (CreateSignupCodeRow, error) {
	return s.Queries.CreateSignupCode(ctx, CreateSignupCodeParams{CodeHash: hash, Note: note, ExpiresAt: time.Now().Add(ttl)})
}
