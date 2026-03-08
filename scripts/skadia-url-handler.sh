#!/bin/bash
# Registriert skadia:// URL-Schema im Desktop
#
# skadia://import?url=https://example.com/ride.fit
# → Browser: http://localhost:5173/import/url?url=https://example.com/ride.fit
#
# Usage: ./scripts/skadia-url-handler.sh [install|uninstall]

DESKTOP_FILE="$HOME/.local/share/applications/skadia-import.desktop"

case "${1:-install}" in
  install)
    mkdir -p "$HOME/.local/share/applications"
    cat > "$DESKTOP_FILE" << 'DESKTOP'
[Desktop Entry]
Type=Application
Name=Skadia Import
Exec=sh -c 'FITURL=$(echo "%u" | sed "s|skadia://import?url=||"); xdg-open "http://localhost:5173/import/url?url=$FITURL"'
MimeType=x-scheme-handler/skadia;
NoDisplay=true
DESKTOP
    xdg-mime default skadia-import.desktop x-scheme-handler/skadia 2>/dev/null || true
    update-desktop-database "$HOME/.local/share/applications/" 2>/dev/null || true
    echo "skadia:// URL-Schema registriert"
    ;;
  uninstall)
    rm -f "$DESKTOP_FILE"
    update-desktop-database "$HOME/.local/share/applications/" 2>/dev/null || true
    echo "skadia:// URL-Schema entfernt"
    ;;
  *)
    echo "Usage: $0 [install|uninstall]"
    exit 1
    ;;
esac
