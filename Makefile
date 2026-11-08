PREFIX ?= $(HOME)/.local
BINDIR ?= $(PREFIX)/bin

.PHONY: build run test lint clean release build-with-token refresh-token install uninstall check-deps

check-deps:
ifeq ($(shell uname -s),Linux)
	@echo "Checking dependencies..."
	@pkg-config --exists gstreamer-1.0 2>/dev/null || \
		{ echo ""; \
		echo "ERROR: gstreamer-1.0 not found."; \
		echo "Please install: gstreamer, gstreamer-plugins-base, gstreamer-plugins-good, and gst-libav (or gstreamer1.0-libav / gstreamer1.0-plugins-bad)"; \
		echo ""; exit 1; }

	@echo "All dependencies found."
endif

# build also refreshes the installed `utuimp` command ($(BINDIR)/utuimp), so
# `make build && ./utuimp` and plain `utuimp` run the same version.
# NO_INSTALL=1 skips that.
build: check-deps
	go build -o utuimp .
ifndef NO_INSTALL
	@install -d "$(BINDIR)"
	@install -m 755 utuimp "$(BINDIR)/utuimp"
	@echo "Updated $(BINDIR)/utuimp"
endif

# build-with-token embeds the Apple developer token without obfuscation.
# Use this for local testing of the embedded-token flow.
# Requires: APPLE_KEY_ID, APPLE_TEAM_ID, APPLE_PRIVATE_KEY
# Optional: LASTFM_API_KEY, LASTFM_API_SECRET (embed Last.fm keys for testing)
build-with-token:
	@test -n "$$APPLE_KEY_ID"      || { echo "APPLE_KEY_ID is not set";      exit 1; }
	@test -n "$$APPLE_TEAM_ID"     || { echo "APPLE_TEAM_ID is not set";     exit 1; }
	@test -n "$$APPLE_PRIVATE_KEY" || { echo "APPLE_PRIVATE_KEY is not set"; exit 1; }
	@set -e; \
	echo "Generating developer token..."; \
	TOKEN=$$(go run ./scripts/gen-devtoken 2>/dev/null); \
	test -n "$$TOKEN" || { echo "gen-devtoken produced no output — check credentials"; exit 1; }; \
	LDFLAGS="-X 'github.com/tphx37/utuimp/internal/auth.devToken=$$TOKEN'"; \
	if [ -n "$$LASTFM_API_KEY" ] && [ -n "$$LASTFM_API_SECRET" ]; then \
		LDFLAGS="$$LDFLAGS -X 'github.com/tphx37/utuimp/internal/lastfm.apiKey=$$LASTFM_API_KEY'"; \
		LDFLAGS="$$LDFLAGS -X 'github.com/tphx37/utuimp/internal/lastfm.apiSecret=$$LASTFM_API_SECRET'"; \
		echo "Embedding Last.fm API keys..."; \
	fi; \
	echo "Building with embedded token..."; \
	go build \
		-ldflags "$$LDFLAGS" \
		-o utuimp . && echo "Done. Run: utuimp auth login"

# release builds a release binary wired to the token broker and Last.fm API keys.
# Requires:
#   UTUIMP_TOKEN_URL, UTUIMP_CLIENT_KEY
#   LASTFM_API_KEY, LASTFM_API_SECRET (from https://www.last.fm/api/account/create)
# Usage: make release UTUIMP_TOKEN_URL=https://.../v1/token UTUIMP_CLIENT_KEY=... \
#                     LASTFM_API_KEY=... LASTFM_API_SECRET=...
release: check-deps
	@test -n "$$UTUIMP_TOKEN_URL"   || { echo "UTUIMP_TOKEN_URL is not set";   exit 1; }
	@test -n "$$UTUIMP_CLIENT_KEY"  || { echo "UTUIMP_CLIENT_KEY is not set";  exit 1; }
	@test -n "$$LASTFM_API_KEY"    || { echo "LASTFM_API_KEY is not set";    exit 1; }
	@test -n "$$LASTFM_API_SECRET" || { echo "LASTFM_API_SECRET is not set"; exit 1; }
	@set -e; \
	echo "Building release binary..."; \
	go build \
		-ldflags "-s -w \
		          -X 'github.com/tphx37/utuimp/internal/auth.tokenURL=$$UTUIMP_TOKEN_URL' \
		          -X 'github.com/tphx37/utuimp/internal/auth.clientKey=$$UTUIMP_CLIENT_KEY' \
		          -X 'github.com/tphx37/utuimp/internal/lastfm.apiKey=$$LASTFM_API_KEY' \
		          -X 'github.com/tphx37/utuimp/internal/lastfm.apiSecret=$$LASTFM_API_SECRET'" \
		-o utuimp . && echo "Done."

# refresh-token generates a fresh Apple developer JWT and writes it into your
# local utuimp config (apple_developer_token). Use it when the token expires
# (~30 days) and you build without an embedded token (plain `make build`).
#
# Credentials are read from the environment or a local .env file (gitignored):
#   APPLE_KEY_ID, APPLE_TEAM_ID
#   APPLE_PRIVATE_KEY       - PEM contents of the .p8 key, OR
#   APPLE_PRIVATE_KEY_FILE  - path to the .p8 key (~ is expanded)
# After refreshing, run `utuimp auth login` if your user token is not set.
refresh-token:
	@set -e; \
	if [ -f .env ]; then set -a; . ./.env; set +a; fi; \
	if [ -z "$$APPLE_PRIVATE_KEY" ] && [ -n "$$APPLE_PRIVATE_KEY_FILE" ]; then \
		APPLE_PRIVATE_KEY="$$(cat "$$(eval echo $$APPLE_PRIVATE_KEY_FILE)")"; \
		export APPLE_PRIVATE_KEY; \
	fi; \
	test -n "$$APPLE_KEY_ID"      || { echo "APPLE_KEY_ID is not set (env or .env)";      exit 1; }; \
	test -n "$$APPLE_TEAM_ID"     || { echo "APPLE_TEAM_ID is not set (env or .env)";     exit 1; }; \
	test -n "$$APPLE_PRIVATE_KEY" || { echo "APPLE_PRIVATE_KEY or APPLE_PRIVATE_KEY_FILE is not set (env or .env)"; exit 1; }; \
	export APPLE_KEY_ID APPLE_TEAM_ID APPLE_PRIVATE_KEY; \
	go run ./scripts/gen-devtoken -write

# install rebuilds with an embedded developer token and copies the binary onto
# PATH, so the installed copy cannot silently go stale behind a newer ./utuimp in
# the working tree. Credentials come from the environment or a local .env
# (gitignored), which may point at the key with APPLE_PRIVATE_KEY_FILE instead
# of inlining its contents as APPLE_PRIVATE_KEY — the same inputs refresh-token
# accepts.
#
# Embedded tokens expire after 30 days, so re-run this when the catalog starts
# returning 401s. Override the location with: make install PREFIX=/usr/local
install:
	@set -e; \
	if [ -f .env ]; then set -a; . ./.env; set +a; fi; \
	if [ -z "$$APPLE_PRIVATE_KEY" ] && [ -n "$$APPLE_PRIVATE_KEY_FILE" ]; then \
		key_file="$$(eval echo $$APPLE_PRIVATE_KEY_FILE)"; \
		test -f "$$key_file" || { echo "APPLE_PRIVATE_KEY_FILE not found: $$key_file"; exit 1; }; \
		APPLE_PRIVATE_KEY="$$(cat "$$key_file")"; \
	fi; \
	export APPLE_KEY_ID APPLE_TEAM_ID APPLE_PRIVATE_KEY LASTFM_API_KEY LASTFM_API_SECRET; \
	if [ -n "$$APPLE_KEY_ID" ] && [ -n "$$APPLE_TEAM_ID" ] && [ -n "$$APPLE_PRIVATE_KEY" ]; then \
		$(MAKE) --no-print-directory build-with-token; \
	else \
		echo "No Apple key in the environment/.env: building without an embedded token"; \
		echo "(utuimp uses apple_developer_token from config, refreshed automatically)."; \
		$(MAKE) --no-print-directory build; \
	fi; \
	install -d "$(BINDIR)"; \
	install -m 755 utuimp "$(BINDIR)/utuimp"; \
	"$(BINDIR)/utuimp" setup-desktop || true; \
	echo "Installed utuimp to $(BINDIR)/utuimp"; \
	case ":$$PATH:" in *":$(BINDIR):"*) ;; *) \
		echo "Note: $(BINDIR) is not on your PATH. Add it, e.g.:"; \
		echo "  fish: fish_add_path $(BINDIR)"; \
		echo "  bash/zsh: echo 'export PATH=\"$(BINDIR):\$$PATH\"' >> ~/.profile";; \
	esac

uninstall:
	rm -f "$(BINDIR)/utuimp"
	rm -f "$(PREFIX)/share/applications/io.github.tphx37.utuimp.desktop"
	rm -f "$(PREFIX)/share/icons/hicolor/scalable/apps/io.github.tphx37.utuimp.svg"

run: build
	./utuimp

test:
	go test ./...

lint:
	golangci-lint run

clean:
	rm -f utuimp
