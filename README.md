<div align="center">
  <img src="docs/hero.svg" width="100%" alt="GoPro Yank — every original, downloaded and verified" />
  <p>
    <a href="https://github.com/azohra/gopro-yank/releases"><img alt="Release" src="https://img.shields.io/github/v/release/azohra/gopro-yank?color=58E0B4"></a>
    <a href="https://github.com/azohra/gopro-yank/blob/main/LICENSE"><img alt="License: MIT" src="https://img.shields.io/badge/license-MIT-F4F0E8"></a>
    <img alt="No runtime required" src="https://img.shields.io/badge/runtime-none-FF5C35">
  </p>
</div>

# GoPro Yank

**Bring your GoPro library home—and know nothing is missing.**

GoPro Yank downloads every original available in your GoPro cloud library,
checks every file, and leaves you with a portable archive you control. No
Python, no developer setup, and no account required to inspect it later.

![GoPro Yank terminal demo](docs/demo.gif)

## Start here

```sh
brew install --cask azohra/tools/gopro-yank
gopro-yank demo
```

Homebrew downloads the ready-to-run app for your computer. `demo` lets you try
the complete flow without connecting an account.

Ready for the real thing?

```sh
gopro-yank login
gopro-yank pull --out ~/Pictures/GoPro
```

That is the whole job. GoPro Yank finds your media, checks disk space,
downloads several originals at once, verifies each one, and writes a readable
report. Stop whenever you like; run the same command to continue.

## What you get

```text
GoPro/
├── originals/          your photos and videos
└── .gopro-yank/
    ├── report.html     a readable result you can keep
    ├── manifest.json   the complete archive index
    ├── checksums.sha256
    ├── snapshots/      saved GoPro library lists
    └── recovery/       prior files kept during repair
```

Your login stays on this computer and is never copied into the archive. File
names are safe across macOS, Windows, and Linux, and ZIP contents cannot escape
the archive directory.

When GoPro Yank prints `DOWNLOADABLE MEDIA EXPORT COMPLETE`, every original it
found in the latest library list has downloaded and passed its file checks.
GoPro does not publish the camera's original checksums, so verification begins
when a file enters your archive. `MultiClipEdit` timelines still need a manual
export.

## Check it anytime

Verification works offline and reads every archived file:

```sh
gopro-yank verify --out ~/Pictures/GoPro
```

You can also compare the archive with your current GoPro library or a second
copy:

```sh
gopro-yank verify --source --out ~/Pictures/GoPro
gopro-yank verify --out ~/Pictures/GoPro --replica /Volumes/Backup/GoPro
```

GoPro Yank never deletes cloud or archived media. If a file is damaged, the
next `pull` verifies its replacement before keeping the prior copy in
`recovery/`.

## Commands

| Command | What it does |
|---|---|
| `login` | Connect your GoPro account |
| `pull` | Download and check every original |
| `verify` | Check the archive, GoPro library, or another copy |
| `status` | Show what is complete and what needs attention |
| `report` | Rebuild or open the offline report |
| `list` | List archived media |
| `manifest` | Print or copy the archive index |
| `skip` | Mark an item for manual handling |
| `demo` | Try the CLI without an account |

Run `gopro-yank <command> --help` for options.

## Other ways to install

Download an archive from [Releases](https://github.com/azohra/gopro-yank/releases),
check it with `checksums.txt`, and put the executable on your `PATH`.

| Computer | Download |
|---|---|
| Apple Silicon Mac | `gopro-yank_darwin_arm64.tar.gz` |
| Intel Mac | `gopro-yank_darwin_amd64.tar.gz` |
| Windows x64 / ARM64 | `gopro-yank_windows_amd64.zip` / `gopro-yank_windows_arm64.zip` |
| Linux x64 / ARM64 | `gopro-yank_linux_amd64.tar.gz` / `gopro-yank_linux_arm64.tar.gz` |

## Coming from Python v0?

Your first `pull` can read the old download records in
`~/.local/share/gopro-yank/state/`. Existing files are checked where they are;
the old records are left untouched.

## Contributing

```sh
make check
make build
make release VERSION=1.0.0
```

The pure-Go release build creates ARM64 and AMD64 downloads for macOS, Windows,
and Linux from one machine.

GoPro Yank uses undocumented GoPro cloud endpoints that may change. Use it only
with your own account. It is an independent open-source project and is not
affiliated with GoPro, Inc.

[Brand system](docs/brand.md) ·
[MIT license](https://github.com/azohra/gopro-yank/blob/main/LICENSE) ·
Inspired by [itsankoff/gopro-plus](https://github.com/itsankoff/gopro-plus)
