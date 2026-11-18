# Apple Music tokens

[← docs](README.md)

The online Apple Music catalog requires your own MusicKit developer token
and an Apple Music subscription. utuimp does not obtain tokens from other
projects. A developer token identifies your MusicKit app; signing in in the
browser gives it a separate user token, saved as `apple_user_token`.

Set these fields in the utuimp config (see [configuration](configuration.md)):

```json
{
  "apple_token_source": "own",
  "apple_developer_token": "YOUR_MUSICKIT_DEVELOPER_JWT"
}
```

Then enable Apple Music in `utuimp setup services` and run `utuimp --apple`.
An existing user-supplied token also works without `apple_token_source`.
The About panel and `utuimp auth status` report the token source.

Previously selected shared credentials are cleared when the config loads.
The old `devtoken.json` cache cannot supply a token. If you switch from a
shared token to your own key, sign in again to authorize your MusicKit app.

## Local Music library on macOS

Your local library remains available without a developer token or sign-in:

```bash
utuimp --local
# Or select the Music app's media folder explicitly:
utuimp --local --music-dir "$HOME/Music/Music/Media"
```

Local files scans `~/Music` recursively by default, including Music and old
iTunes media folders. You can choose another folder with Music folder in
settings or `music_dir` in the config. Supported local files include MP3,
FLAC, M4A, WAV and AIFF. Protected subscription downloads and cloud-only
tracks cannot be played as local files; the online catalog needs MusicKit.

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
