#!/usr/bin/env bash
set -euo pipefail
cd "$(dirname "$0")/.."
make build
install -Dm755 build/ape-player "$HOME/.local/bin/ape-player"
install -Dm644 build/licenses/lucide-LICENSE "$HOME/.local/share/ape-player/licenses/lucide-LICENSE"
install -Dm644 assets/ape-player.svg "$HOME/.local/share/icons/hicolor/scalable/apps/ape-player.svg"
install -Dm644 assets/ape-player.desktop "$HOME/.local/share/applications/ape-player.desktop"
python3 - "$HOME/.local/share/applications/ape-player.desktop" "$HOME/.local/bin/ape-player" <<'PY'
from pathlib import Path
import sys
p=Path(sys.argv[1])
# Desktop Exec quoting, including paths containing spaces.
executable=sys.argv[2].replace('\\','\\\\').replace('"','\\"').replace('`','\\`').replace('$','\\$')
p.write_text(p.read_text().replace('Exec=ape-player','Exec="'+executable+'"'))
PY
if command -v update-desktop-database >/dev/null 2>&1; then
 update-desktop-database "$HOME/.local/share/applications"
fi
