# OpenSSL 1.0.2u (December 2019, end of life) built with everything modern
# builds leave out: SSLv2, SSLv3, EXPORT and LOW ciphers. TEST LAB ONLY:
# these servers are deliberately vulnerable and must never be reachable from
# outside the lab network (see test/tlslab/compose.yaml).
FROM debian:bookworm-slim@sha256:7c7b2c966bc9ee8cedfeef67e0e279108992c77681fa595db4a9d65c06ccc587 AS build
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl gcc libc6-dev make perl \
	&& rm -rf /var/lib/apt/lists/*
ARG OPENSSL_URL=https://www.openssl.org/source/old/1.0.2/openssl-1.0.2u.tar.gz
ARG OPENSSL_SHA256=ecd0c6ffb493dd06707d38b14bb4d8c2288bb7033735606569d8f90f89669d16
RUN curl -fsSLo /tmp/openssl.tar.gz "$OPENSSL_URL" \
	&& echo "$OPENSSL_SHA256  /tmp/openssl.tar.gz" | sha256sum -c - \
	&& tar -xzf /tmp/openssl.tar.gz -C /tmp
WORKDIR /tmp/openssl-1.0.2u
RUN ./config --prefix=/opt/openssl --openssldir=/opt/openssl no-shared no-asm \
		enable-ssl2 enable-ssl3 enable-weak-ssl-ciphers \
	&& make depend >/dev/null 2>&1 && make >/dev/null 2>&1 && make install_sw >/dev/null
# Lab key material, made at build time: RSA 2048 certificate and DH groups
# of 512 (EXPORT), 1024 and 2048 bits.
RUN mkdir /lab && cd /lab && /opt/openssl/bin/openssl req -x509 -newkey rsa:2048 -nodes -days 3650 \
		-subj /CN=tlslab.test -keyout key.pem -out cert.pem 2>/dev/null \
	&& /opt/openssl/bin/openssl dhparam -out dh512.pem 512 2>/dev/null \
	&& /opt/openssl/bin/openssl dhparam -out dh1024.pem 1024 2>/dev/null \
	&& /opt/openssl/bin/openssl dhparam -dsaparam -out dh2048.pem 2048 2>/dev/null \
	&& chmod 444 /lab/*

FROM debian:bookworm-slim@sha256:7c7b2c966bc9ee8cedfeef67e0e279108992c77681fa595db4a9d65c06ccc587
COPY --from=build /opt/openssl/bin/openssl /usr/local/bin/openssl
COPY --from=build /lab /lab
USER 65534:65534
WORKDIR /lab
ENTRYPOINT ["/usr/local/bin/openssl", "s_server", "-accept", "443", "-cert", "/lab/cert.pem", "-key", "/lab/key.pem", "-www"]
