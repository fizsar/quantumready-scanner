# OpenSSL 3.5.9 (LTS): TLS 1.3 with every suite (CCM and CCM_8 included),
# ffdhe and brainpool groups, pure and hybrid ML-KEM; Camellia, ARIA and CCM
# in TLS 1.2. Test lab only.
FROM debian:bookworm-slim@sha256:7c7b2c966bc9ee8cedfeef67e0e279108992c77681fa595db4a9d65c06ccc587 AS build
RUN apt-get update && apt-get install -y --no-install-recommends ca-certificates curl gcc libc6-dev make perl \
	&& rm -rf /var/lib/apt/lists/*
ARG OPENSSL_URL=https://github.com/openssl/openssl/releases/download/openssl-3.5.9/openssl-3.5.9.tar.gz
ARG OPENSSL_SHA256=603f5602e2eef00d77fbd429d34dcd5822bb301757a1bc9cdb24c670f1eb859a
RUN curl -fsSLo /tmp/openssl.tar.gz "$OPENSSL_URL" \
	&& echo "$OPENSSL_SHA256  /tmp/openssl.tar.gz" | sha256sum -c - \
	&& tar -xzf /tmp/openssl.tar.gz -C /tmp
WORKDIR /tmp/openssl-3.5.9
RUN ./Configure --prefix=/opt/openssl --openssldir=/opt/openssl --libdir=lib no-shared no-docs no-tests >/dev/null
RUN make -j"$(nproc)" >/dev/null 2>&1 && make install_sw install_ssldirs >/dev/null
RUN mkdir /lab && cd /lab && /opt/openssl/bin/openssl req -x509 -newkey rsa:2048 -nodes -days 3650 \
		-subj /CN=tlslab.test -keyout key.pem -out cert.pem 2>/dev/null \
	&& chmod 444 /lab/*

FROM debian:bookworm-slim@sha256:7c7b2c966bc9ee8cedfeef67e0e279108992c77681fa595db4a9d65c06ccc587
COPY --from=build /opt/openssl/bin/openssl /usr/local/bin/openssl
COPY --from=build /lab /lab
USER 65534:65534
WORKDIR /lab
ENTRYPOINT ["/usr/local/bin/openssl", "s_server", "-accept", "443", "-cert", "/lab/cert.pem", "-key", "/lab/key.pem", "-www"]
