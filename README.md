# splunk-tui

⚡ A fast, full-featured Terminal UI (TUI) for Splunk written in Go using [Bubble Tea](https://github.com/charmbracelet/bubbletea) and the Splunk REST API.

---

## ✨ Features

- **🗃 Multi-Tab Search Workspaces (`Ctrl+T` / `Ctrl+W`)**:
  - Open multiple independent search workspaces on the same connected instance.
  - Background queries continue polling asynchronously across all tabs.
  - Switch tabs instantly with `[` / `]` (or `Alt+[` / `Alt+]` / `Ctrl+Left` / `Ctrl+Right`) or direct jump via `Alt+1` .. `Alt+9`.
  - Close tabs anytime with `Ctrl+W` (even while the SPL search input is focused).
- **🌐 Multi-View Grid Dashboard**:
  - Connect and search across multiple Splunk instances simultaneously in a unified grid.
  - Fullscreen any instance card with `f` or `Enter` for focused inspection.
  - Synchronized search execution, timepicker, and cancellation (`Esc`) across all cluster nodes.
- **⏱ Interactive Timeline Scrubbing & Zoom-In (`<` / `>` / `z`)**:
  - Scrub bucket-by-bucket across the event density timeline using `<` / `,` and `>` / `.`.
  - Visual cursor indicator (`▲`) highlights the active bucket with event counts and time bounds.
  - Press `z` or `Z` to instantly zoom in and re-execute search within that time window.
  - Press `Esc` to clear the scrub cursor and return to normal view.
- **⚙ Full-Screen Configuration Explorer (`Ctrl+K` / `c`)**:
  - Browse all Splunk `.conf` files (`props.conf`, `transforms.conf`, `inputs.conf`, `server.conf`, etc.).
  - Interactive 3-column browser: Files ⇄ Stanzas ⇄ Key-Value Properties.
  - Filter stanzas and settings in real time (`/`).
  - Viewport windowing handles `.conf` files with hundreds of stanzas smoothly.
- **🎯 Syntax Highlighting & Log Level Coloring**:
  - High-contrast coloring for `FATAL`, `ERROR`, `WARN`, `INFO`, and `DEBUG` logs.
  - Automatic detection and coloring of IPv4 addresses, key-value pairs (`key=value`), timestamps, and quoted strings.
  - Visual severity status badges (`✖`, `⚠`, `●`) in table rows and event inspector.
- **🔍 Instance Browser with Live Filtering**:
  - Manage and connect to known Splunk servers with instant connectivity checks.
  - Press `/` to filter bookmarks in real time across instance names and URLs.
  - Multi-select instances (`Space` or `Ctrl+A`) to launch the Multi-View dashboard.
- **📊 Adaptive Chart & Visualization Pane**:
  - Event density histograms, column charts, bar charts, and time series line/sparkline charts.
  - Cycle chart types with `v` (Bar, Pie, Column, Line, Sparkline).
  - Cycle chart height with `h` / `Ctrl+H` (Small → Medium → Large → Full).
  - Collapse / expand chart pane via `Ctrl+G`.
- **🏷 Smart Fields Sidebar**:
  - Automatic filtering of internal `_*` fields with field coverage percentages.
  - Top mode values with counts and frequencies.
  - Quick inclusion (`+`) or exclusion (`-`) into base SPL query.
  - Toggle fields as extra table columns with `s`.
- **📋 Events & Results Table**:
  - Full pagination (`n` / `p` or `PgDn` / `PgUp`) and horizontal scrolling (`←` / `→`).
  - Press `Enter` to open the full event inspector modal with vertical scrolling (`↑` / `↓`).
- **🔒 Secure Credential Storage**:
  - Passwords stored in OS Keyring (Windows Credential Manager, macOS Keychain, Linux Secret Service).
  - Encrypted file fallback (AES-GCM) with optional master password unlock screen (`keyring_backend: file`).
- **💻 Headless CLI Mode**:
  - Run non-interactive SPL queries from terminal scripts: `./splunk-tui --instance prod --spl '...' --json`.

---

## 🚀 Installation & Build

### Requirements
- Go 1.21+ (Go 1.24 recommended)

### Build Locally
```bash
# Verify dependencies and run tests
go test ./...

# Build binary
go build -o splunk-tui .

# Or using the build script:
./build.sh
```

---

## ⌨ Keyboard Shortcuts

### Search Dashboard & Tabs
| Key | Action |
|---|---|
| `Ctrl+T` | Open new search workspace tab |
| `Ctrl+W` | Close active search tab (works from anywhere, even inside search input) |
| `[` / `]` | Switch to previous / next tab (or `Alt+[` / `Alt+]` / `Ctrl+Left` / `Ctrl+Right`) |
| `Alt+1` .. `Alt+9` | Direct jump to tab 1 through 9 |
| `/` or `Ctrl+F` | Focus SPL search query input |
| `Enter` | Submit search (in input) / Inspect event detail (in table) |
| `Ctrl+E` | Open time range picker (or `Tab` ⇄ `Enter` on time buttons) |
| `Ctrl+X` or `Esc` | Cancel running search job / Clear timeline scrub / Unfocus input |
| `<` / `>` (or `,` / `.`) | Scrub timeline left / right bucket |
| `z` / `Z` | Zoom in to scrubbed timeline slice and re-run search |
| `Ctrl+K` or `c` | Open full-screen Splunk Configuration Explorer |
| `Ctrl+G` | Toggle collapse / expand chart pane |
| `v` | Cycle chart visualization type (Bar, Pie, Column, Line, Spark) |
| `h` or `Ctrl+H` | Cycle chart height (Small → Med → Lrg → Full) |
| `Tab` / `Shift+Tab` | Cycle focus between Search Bar ⇄ Fields ⇄ Table |
| `↑` / `↓` (`k` / `j`) | Navigate rows in results table or fields list |
| `←` / `→` | Scroll table horizontally (or shift columns in transforming mode) |
| `Shift+←` / `Shift+→` | Fast horizontal scroll table (jump 60 chars / 3 cols) |
| `0` / `Home` | Scroll table back to the leftmost column |
| `n` / `p` (or `PgDn` / `PgUp`) | Next / previous results page |
| `+` / `-` | Include / exclude selected field in base search query |
| `s` | Toggle field as an extra table column |
| `Ctrl+O` | Open instance browser to switch servers |
| `?` | Toggle shortcuts help dialog |
| `q` or `Ctrl+C` | Quit application |

### Multi-View Grid Dashboard
| Key | Action |
|---|---|
| `Enter` or `f` | Toggle fullscreen focus for highlighted instance card |
| `Esc` | Exit fullscreen card / Cancel active search jobs |
| `↑` / `↓` / `←` / `→` (`k` / `j` / `h` / `l`) | Navigate instance cards in grid |
| `v` | Cycle chart visualization type across all instances |
| `/` or `Ctrl+F` | Focus global search query input |
| `Ctrl+O` | Return to Instance Browser |
| `q` | Quit application |

### Instance Browser (`Ctrl+O`)
| Key | Action |
|---|---|
| `Enter` | Connect to highlighted instance (or launch Multi-View if 2+ selected) |
| `Space` | Select / deselect instance for Multi-View |
| `Ctrl+A` | Select all instances for Multi-View |
| `/` | Filter instances in real time by name or URL |
| `↑` / `↓` (`k` / `j`) | Navigate instance list |
| `a` / `e` / `d` | Add / Edit / Delete bookmark |
| `Esc` | Clear filter / return to active session |

### Configuration Explorer (`Ctrl+K` or `c`)
| Key | Action |
|---|---|
| `Tab` / `Shift+Tab` | Switch between Files ⇄ Stanzas ⇄ Properties panes |
| `←` / `→` (`h` / `l`) | Move focus left / right across panes |
| `↑` / `↓` (`k` / `j`) | Navigate items in active pane |
| `PgUp` / `PgDn` | Fast scroll (jump 10 items) |
| `Enter` | Load stanzas for selected file / inspect properties |
| `/` | Filter stanzas or properties in real time |
| `Esc` or `q` | Back to previous pane / exit explorer |

### Event Inspector Modal (`Enter` on Table Row)
| Key | Action |
|---|---|
| `↑` / `↓` (`k` / `j`) | Scroll through fields and syntax-highlighted `_raw` log |
| `Esc` / `Enter` / `q` | Close event inspector |

---

## 🛠 Configuration & Secret Storage

### File Locations
Configuration files and encrypted credentials are kept strictly in user-level directories:
- **Windows**:
  - Config: `%APPDATA%\splunk-tui\config.yaml`
  - File Keyring: `%APPDATA%\splunk-tui\keyring\`
- **Linux**:
  - Config: `~/.config/splunk-tui/config.yaml`
  - File Keyring: `~/.config/splunk-tui/keyring/`
- **macOS**:
  - Config: `~/Library/Application Support/splunk-tui/config.yaml`
  - File Keyring: `~/Library/Application Support/splunk-tui/keyring/`

### Example `config.yaml`
Passwords and tokens are **never** written to `config.yaml`:
```yaml
page_size: 50
default_time_range: -24h
keyring_backend: auto # auto, wincred, keychain, secret-service, or file
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

### Keyring Backends & Master Password
- **`auto`** (default): Automatically chooses the best OS-level keyring:
  - Windows: Windows Credential Manager (`wincred`)
  - macOS: Keychain (`keychain`)
  - Linux: Secret Service API via DBus (`secret-service`)
- **`file`**: Uses an AES-GCM encrypted file store saved in `<config-dir>/keyring/`.
  - When using `file`, you are prompted on startup to enter a master password to unlock the keyring.
  - In headless scripts or CI/CD pipelines, set the `SPLUNK_TUI_KEYRING_PASSWORD` environment variable to unlock automatically without human intervention.

---

## 💻 CLI Usage

### 1. Interactive TUI
```bash
# Launch instance browser:
./splunk-tui

# Auto-connect directly to a saved bookmark:
./splunk-tui --instance prod-search
```

### 2. Scripting & Automation (Non-Interactive)
```bash
# Run SPL query and stream formatted events to terminal:
./splunk-tui --instance prod-search --spl 'index=_internal sourcetype=splunkd | head 20'

# Output raw JSON to pipe into jq:
./splunk-tui --instance prod-search --spl '| stats count by sourcetype' --json | jq .

# Custom time range and max count:
./splunk-tui --instance prod-search --spl 'index=main error' --earliest -1h --latest now --count 100
```

---

## 🔒 Security & Git Safety

### Zero Repository Exposure
- **No Secrets in Repo**: Passwords, bearer tokens, and session keys are never stored inside this repository.
- **Protected by `.gitignore`**: The repository explicitly ignores local configuration files (`config.yaml`), local keyring files (`keyring/`), environment files (`.env*`), tokens (`*.token`), and certificates (`*.pem`, `*.key`).
- **Safe to Push**: Pushing to GitHub or public remotes carries zero risk of leaking your saved Splunk credentials or server configurations.

### Headless & CI/CD Best Practices
- For automated testing or CLI runs in CI pipelines (e.g. GitHub Actions), supply credentials via environment variables (`SPLUNK_TUI_KEYRING_PASSWORD`) or configure temporary test tokens without committing credentials to version control.
