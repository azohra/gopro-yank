<div align="center">
  <img src="docs/hero.svg" width="100%" alt="GoPro Yank — bring your GoPro library home" />
  <p>
    <a href="https://github.com/azohra/gopro-yank/releases"><img alt="Release" src="https://img.shields.io/github/v/release/azohra/gopro-yank?color=58E0B4"></a>
    <a href="https://github.com/azohra/gopro-yank/blob/main/LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/license-MIT-F4F0E8"></a>
    <img alt="No runtime required" src="https://img.shields.io/badge/runtime-none-FF5C35">
  </p>
</div>

# GoPro Yank

**Bring your GoPro library home.**

[Website](https://gopro-yank.azohra.com) · [Latest release](https://github.com/azohra/gopro-yank/releases/latest)

GoPro Yank downloads every available original from your GoPro cloud library,
checks every saved file, and gives you a portable archive you control. It never
deletes GoPro cloud media.

![GoPro Yank interactive app](docs/demo.gif)

## Get started

```sh
brew install --cask azohra/tools/gopro-yank
gopro-yank
```

That opens the interactive app. Connect your GoPro account, look through the
size and shape of your library, choose a folder, and confirm when you are ready
to archive it.

The library view is read-only. **Nothing downloads until you choose Archive and
confirm it.**

Want to look around first? `gopro-yank --demo` runs the whole experience with
sample data and no account.

## What happens

1. GoPro Yank opens GoPro's website in a separate browser window. You sign in
   there, so the app never sees your password.
2. It reads your library and shows what is already archived, which originals
   can download, and what still needs a manual export.
3. After you confirm, it checks free space, downloads the available originals,
   and verifies every saved file.
4. It leaves a readable offline report beside your archive records.

You can stop safely at any time. Finished files stay finished; open GoPro Yank
again to continue.

## Your archive

By default, GoPro Yank uses `Pictures/GoPro` on macOS and Windows. On Linux it
uses `GoPro-Archive` in the current folder. You can choose any local folder or
mounted external drive before archiving.

```text
GoPro/
├── originals/          your photos and videos
└── .gopro-yank/
    ├── report.html     a readable result you can keep
    ├── manifest.json   the archive index
    ├── checksums.sha256
    ├── snapshots/      dated GoPro library lists
    └── recovery/       prior files kept during repair
```

Your GoPro login stays on this computer and is never copied into the archive.
The archive works across macOS, Windows, and Linux.

To remove an archive from your computer, open GoPro Yank and choose **Delete
local archive**. It requires an explicit `DELETE` confirmation, never touches
GoPro cloud media, and leaves unrelated files and the archive folder itself in
place.

When GoPro Yank says `DOWNLOADABLE MEDIA EXPORT COMPLETE`, every downloadable
original in the latest library list is present and has passed its checks. GoPro
does not provide the camera's original checksums, so GoPro Yank records a
checksum as each file enters your archive and uses it for every later check.
GoPro `MultiClipEdit` timelines still require a manual export.

## Command-line use

The interactive app is the normal way in. These commands expose the same core
operations without the interface:

| Command | What it does |
|---|---|
| `gopro-yank library` | Inspect the GoPro library without downloading |
| `gopro-yank archive` | Archive or resume every available original |
| `gopro-yank verify` | Check every archived file offline |
| `gopro-yank login` | Connect without opening the interactive app |

Run `gopro-yank <command> --help` for options. For example:

```sh
gopro-yank archive --out /Volumes/Photos/GoPro
gopro-yank verify --out /Volumes/Photos/GoPro
gopro-yank verify --out /Volumes/Photos/GoPro --replica /Volumes/Backup/GoPro
```

On a computer without a browser, use `gopro-yank login --no-browser` and follow
the prompt. Environment-based setups can start from
[`.env.example`](.env.example).

Older commands still work for existing scripts, including `pull` as an alias
for `archive`.

## Other ways to install

Download the package for your computer from
[Releases](https://github.com/azohra/gopro-yank/releases), check it against
`checksums.txt`, and put `gopro-yank` on your `PATH`.

| Computer | Package |
|---|---|
| Apple Silicon Mac | `gopro-yank_darwin_arm64.tar.gz` |
| Intel Mac | `gopro-yank_darwin_amd64.tar.gz` |
| Windows x64 / ARM64 | `gopro-yank_windows_amd64.zip` / `gopro-yank_windows_arm64.zip` |
| Linux x64 / ARM64 | `gopro-yank_linux_amd64.tar.gz` / `gopro-yank_linux_arm64.tar.gz` |

Each package contains one executable and the license. Python and Go are not
required.

## Coming from Python v0?

Your first archive run can read the old download records in
`~/.local/share/gopro-yank/state/`. Existing files are checked where they are;
the old records are left untouched.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

GoPro Yank uses undocumented GoPro cloud endpoints that may change. Use it only
with your own account. It is an independent open-source project and is not
affiliated with GoPro, Inc.

[Brand system](docs/brand.md) ·
[MIT license](https://github.com/azohra/gopro-yank/blob/main/LICENSE) ·
Inspired by [itsankoff/gopro-plus](https://github.com/itsankoff/gopro-plus)
