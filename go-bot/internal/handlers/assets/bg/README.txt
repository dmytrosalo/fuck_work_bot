Table background photos.

Drop .jpg / .jpeg / .png / .webp files in this directory and they are served
as random poker table backgrounds — no code change needed, the list is built
from whatever is embedded at build time.

They are compiled INTO the binary (go:embed), so keep them reasonably sized:
about 1200-1600px wide and a few hundred KB each. A dozen large photos will
bloat the image and slow every deploy.

This README exists so the embed pattern always matches something and the
build never breaks on an empty directory.
