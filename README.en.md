# caixin2kindle

Download a 《Caixin Weekly》issue and push it to your Kindle.

```
caixin2kindle https://weekly.caixin.com/2026/cw1224/
```

## Quick start

| Item | Details |
|---|---|
| Language | Go 1.27 |
| Platform | macOS |
| Dependencies | Chromium browser, calibre (`ebook-convert`) |
| Output | EPUB + MOBI in `<out>/<issue>/` |

## Install

```bash
go build -o caixin2kindle .
```

Make sure `ebook-convert` is on PATH and a Chromium browser is available.

## Usage

```
caixin2kindle [flags] <issue URL>

Flags:
  --out <dir>          Parent output directory (default ~/Downloads/caixin)
  --kindle <mount>     Kindle mount point (default /Volumes/Kindle)
  --browser <path>     Browser executable path (auto-detected by default)
  --delay "M-N"        Random delay between articles in seconds (default "3-8")
  --no-kindle          Produce EPUB + MOBI only, skip Kindle copy
  --full               Full rebuild, ignore existing progress
  --headless           Run browser headless
  -h, --help           Show help
```

## How it works

1. Launch browser → visit issue page → parse article list
2. Click to expand each article → extract full text
3. Assemble EPUB → convert to MOBI via `ebook-convert` → copy to Kindle

First run requires manual login (profile persisted at `~/.caixin/browser-profile`). Do not interact with the browser window while running.

## Exit codes

| Code | Meaning |
|---|---|
| 0 | Success |
| 1 | Bad args / missing dependency / browser startup failure |
| 2 | Fetch failure |
| 3 | Build failure |
| 4 | Kindle detection failure |
| 5 | Other error |

## License

MIT
