# splunk-tui

⚡ A Terminal UI (TUI) for Splunk written in Go using [Bubble Tea](https://github.com/charmbracelet/bubbletea) and the Splunk REST API.

---

## Features

- **Instance Browser**: Manage and connect to known Splunk instances with instant connectivity checks.
- **Secure Password Storage**: Passwords stored via [99designs/keyring](https://github.com/99designs/keyring) (OS Keyring / Windows Credential Manager / macOS Keychain / Secret Service, with an encrypted-file fallback for WSL/headless environments).
- **Interactive SPL Search Bar**: Write SPL with relative time range selection (Last 15m, 60m, 4h, 24h, 7d, 30d, All time).
- **Visualization Chart Pane**:
  - Event density histogram over time for standard searches.
  - Bar charts for transforming commands (`| stats`, `| chart`, `| top`).
  - Time series line charts and sparklines for `| timechart`.
  - Collapsible via `Ctrl+G` to maximize table area.
- **Data Gap Timeline**: Visual activity bar that highlights gaps in log data.
- **Fields Sidebar**:
  - Automatic filtering of internal `_*` fields.
  - Field coverage percentage (e.g. `host  100%`, `status  98%`).
  - Expandable top values (modes) with counts and percentages.
  - Shortcut `+` to include `field="value"` into the base search.
  - Shortcut `-` to exclude `field!="value"` from the base search.
  - Shortcut `s` to toggle field as an extra table column.
- **Events & Results Table**:
  - Full pagination (`n` / `p`).
  - Automatically displays `_time`, selected fields, and `_raw`.
  - Supports `| table col1 col2` to display custom tabular columns.
  - Press `Enter` to open the full event inspector modal.
- **CLI Flags**: `--instance <name>` for immediate auto-connect from terminal scripts or shortcuts.

---

## Installation & Build

### Requirements
- Go 1.21+ (Go 1.24+ recommended)

### Build
From the project directory (or WSL shell):
```bash
# Download and verify dependencies
go mod tidy

# Run unit tests
go test ./...

# Build binary
go build -o splunk-tui .
```

---

## Usage

### 1. Interactive Mode
Run `splunk-tui` without arguments to launch the instance browser:
```bash
./splunk-tui
```

### 2. Auto-connect to a Known Instance
```bash
./splunk-tui --instance prod-search
```

### 3. Non-Interactive CLI Search (`--instance`, `--spl`, and `--json`)
Run queries directly from the CLI without entering the TUI (`--instance` is required):
```bash
# Print raw log events with timestamps:
./splunk-tui --instance prod-search --spl 'index=_internal sourcetype=splunkd | head 20'

# Output results formatted as JSON:
./splunk-tui --instance prod-search --spl 'index=_internal sourcetype=splunkd | head 20' --json

# Query with custom time range:
./splunk-tui --instance prod-search --spl 'index=main error' --earliest -1h --latest now

# Pipe transforming results into jq:
./splunk-tui --instance prod-search --spl '| stats count by sourcetype' --json | jq .
```

### 4. Custom Config or Keyring Backend
```bash
./splunk-tui --config ~/.my-splunk-config.yaml --keyring-backend file
```

---

## Keyboard Shortcuts

| Shortcut | Action |
|---|---|
| `/` or `Ctrl+F` | Focus SPL search input |
| `Enter` | Run search (in input) / Inspect event detail (in table) |
| `Ctrl+T` | Open time range picker |
| `Ctrl+X` | Cancel running search job |
| `Ctrl+G` | Toggle collapse / expand chart pane |
| `Tab` | Cycle focus between Search Bar ⇄ Fields Sidebar ⇄ Table |
| `Shift+Tab` | Cycle focus backwards |
| `n` or `PgDn` | Next results page |
| `p` or `PgUp` | Previous results page |
| `+` | Include field value into base search (`field="val"`) |
| `-` | Exclude field value from base search (`field!="val"`) |
| `s` | Toggle field as an extra column in the table |
| `Ctrl+O` | Open instance browser (switch servers with full UI reset) |
| `?` | Toggle help dialog |
| `q` or `Ctrl+C` | Quit application |

---

## Configuration File

Bookmarks and settings are saved at `~/.config/splunk-tui/config.yaml` (mode `0600`). Passwords are never written to this file in plain text.

```yaml
page_size: 50
default_time_range: -24h
keyring_backend: auto
bookmarks:
  - name: prod-search
    url: https://splunk.corp:8089
    username: admin
    verify_ssl: true
  - name: dev-lab
    url: https://10.0.0.5:8089
    username: admin
    verify_ssl: false
```
