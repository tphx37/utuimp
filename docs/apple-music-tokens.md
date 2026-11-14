# Apple Music tokens

[← docs](README.md)

**utuimp has no Apple Music developer token of its own.** Making one takes
a paid Apple Developer account, which this project doesn't have. Apple Music
in utuimp runs on the developer token of another project,
[vibez](https://github.com/simonepelosi/vibez) (the one utuimp started
from), or on your own key if you have one. If vibez stops handing out
tokens, Apple Music stops working in utuimp until you use your own key.

Apple Music needs two tokens:

- a **developer token**, which identifies the app to Apple. It is the same
  for everyone and is valid for a few days or weeks. This is the borrowed
  one.
- a **user token**, which is your Apple ID's permission for that app. You
  get it by signing in in the browser window utuimp opens the first time.
  It is saved as `apple_user_token` in the config.

## Whose developer token is used

The settings (`,`, or `utuimp setup services`) ask before Apple Music can
be turned on, and save the answer as `apple_token_source` in the config:

| `apple_token_source` | meaning |
|---|---|
| `"vibez"` | borrow the token from vibez (below) |
| `"own"` | only use `apple_developer_token`, made from your own MusicKit key |
| not set | not asked yet: a token that is already saved keeps working, but no new one is fetched |

The About panel and `utuimp auth status` say which one is in use and how
long it has left.

## How the vibez token is fetched

Only with `apple_token_source: "vibez"`, and only when the saved token has
less than three days left:

1. utuimp downloads the latest vibez release for your system from GitHub
   (once a week at most), checks it against the release's `checksums.txt`,
   and keeps only the executable, in `~/.cache/utuimp/vibez/`.
2. It runs it in a throwaway home directory with no access to your config,
   your browser or other programs.
3. As soon as vibez saves its token, utuimp stops it, takes the token and
   saves it in `~/.cache/utuimp/devtoken.json`.

If this fails, the Apple Music tab says why. Usually it is the network or
GitHub, and restarting utuimp later helps. It can also mean vibez changed
or stopped its token service; then only your own key works.

## utuimp's own token service (not in use)

The repository has a small Cloudflare Worker that signs short-lived tokens
([worker/README.md](../worker/README.md)), and release builds can be pointed
at it. It needs an Apple Developer key to sign with, so it is not deployed
and release builds are made without it.

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
