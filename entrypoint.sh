#!/bin/sh
set -e

# Ensure MPD's runtime/data dirs exist and are owned correctly
mkdir -p /var/lib/mpd/music /var/lib/mpd/playlists /var/run/mpd
chown -R bot:bot /var/lib/mpd /var/run/mpd /var/log/mpd 2>/dev/null || true

# Start MPD in the foreground's background (daemonized by mpd itself,
# but we wait on it below) using the config baked into the image.
su -s /bin/sh bot -c "mpd --no-daemon /etc/mpd.conf &"

# Give MPD a moment to create its socket before the bot tries to connect
for i in $(seq 1 20); do
    if mpc -h /var/run/mpd/socket status >/dev/null 2>&1; then
        break
    fi
    sleep 0.5
done

# Hand off to the bot as the final foreground process (PID 1 successor)
exec su -s /bin/sh bot -c "./bot -config /app/config/config.yaml"