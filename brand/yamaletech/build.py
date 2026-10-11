"""Derive every raster the chain apps use from the Yamale Tech lockup.

The source is the company site's logo (yamaletech-site, src/assets/
yamale-logo-new.png): the gold ribbon Y over "YAMALE TECH". It exists only as
a raster, so everything here is a raster too, cut and scaled from that one
file, and never redrawn.

    python brand/yamaletech/build.py      (needs Pillow)

Writes into brand/yamaletech/ and copies the files each app serves into its
public directory. Re-run it whenever the source logo changes.
"""
import os
import shutil

from PIL import Image

HERE = os.path.dirname(os.path.abspath(__file__))
ROOT = os.path.normpath(os.path.join(HERE, "..", ".."))
SRC = os.path.join(HERE, "yamale-tech-lockup.png")

# The opaque ground for the icons a platform will not show transparent (an iOS
# home-screen icon fills transparency with black): the company site's chrome.
GROUND = (24, 24, 24, 255)


def the_y(lockup):
    """The ribbon Y alone: everything above the first fully empty row band."""
    alpha = lockup.getchannel("A")
    w, h = lockup.size
    rows = [alpha.crop((0, y, w, y + 1)).getextrema()[1] for y in range(h)]
    y = 0
    while y < h and rows[y] <= 8:
        y += 1
    while y < h and rows[y] > 8:
        y += 1
    y_mark = lockup.crop((0, 0, w, y))
    return y_mark.crop(y_mark.getchannel("A").point(lambda a: 255 if a > 8 else 0).getbbox())


def square(img, size, pad=0.06, ground=None):
    """img centred on a size x size canvas, with pad of the side left clear."""
    inner = round(size * (1 - 2 * pad))
    scale = inner / max(img.size)
    fitted = img.resize((max(1, round(img.width * scale)), max(1, round(img.height * scale))), Image.LANCZOS)
    canvas = Image.new("RGBA", (size, size), ground or (0, 0, 0, 0))
    canvas.alpha_composite(fitted, ((size - fitted.width) // 2, (size - fitted.height) // 2))
    return canvas


def main():
    lockup = Image.open(SRC).convert("RGBA")
    y = the_y(lockup)
    out = {}

    out["mark-512.png"] = square(y, 512)
    out["mark-256.png"] = square(y, 256, pad=0.02)
    out["mark-64.png"] = square(y, 64, pad=0.02)
    # PWA and home-screen icons: on the dark ground, with room for the masks
    # platforms cut them with.
    out["icon-192.png"] = square(y, 192, pad=0.16, ground=GROUND)
    out["icon-512.png"] = square(y, 512, pad=0.16, ground=GROUND)
    out["apple-touch-icon.png"] = square(y, 180, pad=0.14, ground=GROUND)
    # Android crops a maskable icon to a circle: the Y stays inside the middle 60%.
    out["icon-maskable-512.png"] = square(y, 512, pad=0.24, ground=GROUND)
    for name, img in out.items():
        img.save(os.path.join(HERE, name), optimize=True)

    # Browser tabs: the Y nearly edge to edge, since at 16 px every pixel counts.
    ico = os.path.join(HERE, "favicon.ico")
    square(y, 256, pad=0.02).save(ico, sizes=[(16, 16), (32, 32), (48, 48), (64, 64)])

    # What each app serves. The single-file consoles have no public directory:
    # they load /mark.png from the site root, which every subdomain proxies.
    publics = [
        "clients/site",
        "clients/demo",
        "clients/app/public",
        "clients/explorer/public",
        "clients/wallet/public",
        "clients/safe/public",
        "clients/rwa/public",
        "clients/herd/public",
    ]
    for p in publics:
        d = os.path.join(ROOT, p)
        shutil.copyfile(os.path.join(HERE, "mark-256.png"), os.path.join(d, "mark.png"))
        shutil.copyfile(ico, os.path.join(d, "favicon.ico"))
    for name in ("icon-192.png", "icon-512.png", "icon-maskable-512.png", "apple-touch-icon.png"):
        shutil.copyfile(os.path.join(HERE, name), os.path.join(ROOT, "clients/app/public", name))
    print("Y cut at", y.size, "- wrote", len(out) + 1, "files and", len(publics), "app copies")


if __name__ == "__main__":
    main()
