# Apple Music tokens

[← docs](README.md)

Apple Music needs two tokens:

- a **developer token**, which identifies the app to Apple. It is the same
  for everyone and is valid for a few days or weeks.
- a **user token**, which is your Apple ID's permission for that app. You
  get it by signing in in the browser window utuimp opens the first time.
  It is saved as `apple_user_token` in the config.

You only ever deal with the second one. The developer token is utuimp's
business. This page explains where it comes from, for when something goes
wrong or you build utuimp yourself.

## Where the developer token comes from

utuimp looks in this order and takes the first token that is still valid
for at least three days:

1. **Built into the binary.** Release builds ask the project's token
   service, a small Cloudflare Worker that signs short-lived tokens (see
   [worker/README.md](../worker/README.md)). `make install` instead builds
   in a token made from your own MusicKit key, when it finds one (see
   below).
2. **`apple_developer_token` in the config.** `make refresh-token` writes
   one from your own key.
3. **The last saved token**, in `~/.cache/utuimp/devtoken.json`.
4. **A fresh one from [vibez](https://github.com/simonepelosi/vibez)**, the
   project utuimp started from, whose releases can fetch a developer token.
   utuimp downloads the latest vibez release for your system from GitHub
   (once a week at most), checks it against the release's `checksums.txt`,
   and keeps only the executable, in `~/.cache/utuimp/upstream/`. It runs it
   in a throwaway home directory with no access to your config, your
   browser or other programs. As soon as vibez saves its token, utuimp stops
   it, takes the token and saves it in `devtoken.json`. If GitHub is
   briefly unavailable, utuimp retries a couple of times.

If every step fails, the Apple Music tab says why. Usually it is the
network or GitHub, and restarting utuimp later helps. If Apple rejects the
token outright, utuimp checks for an update right away instead of waiting a
day.

## Using your own MusicKit key

With an Apple Developer account you can make your own key
(Certificates, Identifiers & Profiles, then Keys, with MusicKit enabled, at [developer.apple.com/account](https://developer.apple.com/account)).
Put its details in the environment or in a `.env` file in the repository
(it is gitignored):

```bash
APPLE_KEY_ID=…
APPLE_TEAM_ID=…
APPLE_PRIVATE_KEY_FILE=~/keys/AuthKey_XXXX.p8   # or APPLE_PRIVATE_KEY with the PEM contents
```

Then either:

```bash
make install          # builds the token into the binary
make refresh-token    # writes apple_developer_token into your config
```

A token made this way lasts about 30 days. Run `make refresh-token` again
when it runs out.

## Signing in again

```bash
utuimp --relogin=apple
```

clears the user token and opens the sign-in page again.
