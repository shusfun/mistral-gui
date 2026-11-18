"""Render modern mistral brand icons (squircle app icon + crisp monochrome tray glyphs).

Requires Pillow. Offline & deterministic; SVG + mathematical superellipse / Bezier SSAA renderer.
"""
from pathlib import Path
import math
from PIL import Image, ImageChops, ImageDraw, ImageFilter

ROOT = Path(__file__).resolve().parents[1]
ASSETS = ROOT / "internal/branding/assets"


def superellipse_points(cx, cy, rx, ry, n=4.5, steps=360):
    pts = []
    for i in range(steps):
        theta = (2.0 * math.pi * i) / steps
        ct, st = math.cos(theta), math.sin(theta)
        x = cx + rx * (1 if ct >= 0 else -1) * (abs(ct) ** (2.0 / n))
        y = cy + ry * (1 if st >= 0 else -1) * (abs(st) ** (2.0 / n))
        pts.append((x, y))
    return pts


def linear_gradient(size, p0, p1, stops):
    """Create an RGBA linear gradient image across p0 -> p1 with [(pos, (r,g,b,a)), ...]."""
    w, h = size
    # Render at 1/4 resolution and upscale bicubic for fast, ultra-smooth 2048px gradients
    sw, sh = max(1, w // 4), max(1, h // 4)
    sx0, sy0 = p0[0] / 4.0, p0[1] / 4.0
    sx1, sy1 = p1[0] / 4.0, p1[1] / 4.0
    dx, dy = sx1 - sx0, sy1 - sy0
    denom = dx * dx + dy * dy or 1.0

    small = Image.new("RGBA", (sw, sh))
    px = small.load()
    for y in range(sh):
        for x in range(sw):
            t = ((x - sx0) * dx + (y - sy0) * dy) / denom
            t = 0.0 if t < 0.0 else (1.0 if t > 1.0 else t)
            c = stops[-1][1]
            for idx in range(len(stops) - 1):
                t0, c0 = stops[idx]
                t1, c1 = stops[idx + 1]
                if t <= t1:
                    u = 0.0 if t1 == t0 else (t - t0) / (t1 - t0)
                    c = tuple(int(round(c0[k] + (c1[k] - c0[k]) * u)) for k in range(4))
                    break
            px[x, y] = c
    return small.resize((w, h), Image.Resampling.BICUBIC)


def draw_thick_capsule(mask_draw, p0, p1, radius, fill=255):
    x0, y0 = p0
    x1, y1 = p1
    dx, dy = x1 - x0, y1 - y0
    length = math.hypot(dx, dy)
    if length == 0:
        mask_draw.ellipse((x0 - radius, y0 - radius, x0 + radius, y0 + radius), fill=fill)
        return
    nx, ny = -dy / length * radius, dx / length * radius
    poly = [(x0 + nx, y0 + ny), (x1 + nx, y1 + ny), (x1 - nx, y1 - ny), (x0 - nx, y0 - ny)]
    mask_draw.polygon(poly, fill=fill)
    mask_draw.ellipse((x0 - radius, y0 - radius, x0 + radius, y0 + radius), fill=fill)
    mask_draw.ellipse((x1 - radius, y1 - radius, x1 + radius, y1 + radius), fill=fill)


def build_glyph_masks(S, tray_mode=False):
    """Build left-wing and right-wing masks of the modern aerodynamic 'M' mark."""
    s = S / 512.0
    if tray_mode:
        r = 34.0 * s
        left_top = (102 * s, 118 * s)
        left_bot = (102 * s, 398 * s)
        mid_valley = (256 * s, 276 * s)
        right_top = (410 * s, 118 * s)
        right_bot = (410 * s, 398 * s)
        seam_gap = 18.0 * s
    else:
        r = 30.0 * s
        left_top = (146 * s, 160 * s)
        left_bot = (146 * s, 352 * s)
        mid_valley = (256 * s, 272 * s)
        right_top = (366 * s, 160 * s)
        right_bot = (366 * s, 352 * s)
        seam_gap = 13.5 * s

    left_mask = Image.new("L", (S, S), 0)
    ld = ImageDraw.Draw(left_mask)
    draw_thick_capsule(ld, left_bot, left_top, r, 255)
    draw_thick_capsule(ld, left_top, mid_valley, r, 255)

    right_mask = Image.new("L", (S, S), 0)
    rd = ImageDraw.Draw(right_mask)
    draw_thick_capsule(rd, mid_valley, right_top, r, 255)
    draw_thick_capsule(rd, right_top, right_bot, r, 255)

    # Carve a crisp negative-space overlap seam on left_mask around right_mask's ascending wing
    cutout = Image.new("L", (S, S), 0)
    cd = ImageDraw.Draw(cutout)
    draw_thick_capsule(cd, mid_valley, right_top, r + seam_gap, 255)
    left_mask = ImageChops.subtract(left_mask, cutout)

    return left_mask, right_mask


def render_app_icon():
    S = 2048
    s = S / 512.0
    canvas = Image.new("RGBA", (S, S), (0, 0, 0, 0))

    # 1. Squircle mask
    sq_pts = superellipse_points(256 * s, 256 * s, 228 * s, 228 * s, n=4.5, steps=512)
    sq_mask = Image.new("L", (S, S), 0)
    ImageDraw.Draw(sq_mask).polygon(sq_pts, fill=255)

    # 2. Deep ocean-slate gradient base
    bg_grad = linear_gradient(
        (S, S),
        (64 * s, 36 * s),
        (448 * s, 476 * s),
        [
            (0.0, (20, 54, 86, 255)),
            (0.52, (13, 34, 56, 255)),
            (1.0, (8, 20, 35, 255)),
        ],
    )
    canvas.paste(bg_grad, (0, 0), sq_mask)

    # 3. Subtle top cyan ambient glow
    glow = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    gd = ImageDraw.Draw(glow)
    gd.ellipse((76 * s, -40 * s, 436 * s, 310 * s), fill=(45, 212, 191, 52))
    glow = glow.filter(ImageFilter.GaussianBlur(radius=68 * s))
    glow_clipped = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    glow_clipped.paste(glow, (0, 0), sq_mask)
    canvas = Image.alpha_composite(canvas, glow_clipped)

    # 4. Crisp inner specular rim
    rim_layer = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    rim_pts = superellipse_points(256 * s, 256 * s, 226.2 * s, 226.2 * s, n=4.5, steps=512)
    ImageDraw.Draw(rim_layer).line(rim_pts + [rim_pts[0]], fill=(186, 230, 253, 56), width=int(3.2 * s))
    canvas = Image.alpha_composite(canvas, rim_layer)

    # 5. Glyph masks + subtle drop shadow for depth
    left_mask, right_mask = build_glyph_masks(S, tray_mode=False)
    combined_glyph = ImageChops.lighter(left_mask, right_mask)

    shadow = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    shadow_alpha = combined_glyph.filter(ImageFilter.GaussianBlur(radius=14 * s))
    shadow_solid = Image.new("RGBA", (S, S), (3, 10, 20, 115))
    shadow.paste(shadow_solid, (0, int(10 * s)), shadow_alpha)
    shadow_clipped = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    shadow_clipped.paste(shadow, (0, 0), sq_mask)
    canvas = Image.alpha_composite(canvas, shadow_clipped)

    # 6. Left wing (Sky-Cyan luminous gradient)
    left_grad = linear_gradient(
        (S, S),
        (120 * s, 140 * s),
        (272 * s, 372 * s),
        [
            (0.0, (125, 211, 252, 255)),
            (1.0, (14, 165, 233, 255)),
        ],
    )
    left_layer = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    left_layer.paste(left_grad, (0, 0), left_mask)
    canvas = Image.alpha_composite(canvas, left_layer)

    # 7. Right wing (Bright Mint-Teal gradient)
    right_grad = linear_gradient(
        (S, S),
        (240 * s, 140 * s),
        (396 * s, 372 * s),
        [
            (0.0, (167, 243, 208, 255)),
            (0.48, (45, 212, 191, 255)),
            (1.0, (13, 148, 136, 255)),
        ],
    )
    right_layer = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    right_layer.paste(right_grad, (0, 0), right_mask)
    canvas = Image.alpha_composite(canvas, right_layer)

    return canvas.resize((1024, 1024), Image.Resampling.LANCZOS)


def render_tray_icon(hex_color):
    """Render crisp transparent-background monochrome glyph for 16px/24px OS tray & menu bar."""
    S = 512
    left_mask, right_mask = build_glyph_masks(S, tray_mode=True)
    combined = ImageChops.lighter(left_mask, right_mask)

    hex_clean = hex_color.lstrip("#")
    rgb = tuple(int(hex_clean[i : i + 2], 16) for i in (0, 2, 4))
    out = Image.new("RGBA", (S, S), (0, 0, 0, 0))
    solid = Image.new("RGBA", (S, S), (*rgb, 255))
    out.paste(solid, (0, 0), combined)
    return out.resize((64, 64), Image.Resampling.LANCZOS)


def main():
    (ROOT / "dist").mkdir(exist_ok=True)
    ASSETS.mkdir(parents=True, exist_ok=True)

    im = render_app_icon()
    im.save(ASSETS / "logo.png")
    im.save(ASSETS / "app.ico", sizes=[(n, n) for n in (16, 24, 32, 48, 64, 128, 256)])
    im.save(ASSETS / "app.icns")

    for name, color in [
        ("tray-dark", "#111827"),
        ("tray-light", "#F8FAFC"),
        ("tray-template", "#000000"),
    ]:
        render_tray_icon(color).save(ASSETS / f"{name}.png")

    public = ROOT / "web/public"
    public.mkdir(exist_ok=True)
    im.resize((256, 256), Image.Resampling.LANCZOS).save(public / "logo.png")
    im.save(public / "favicon.ico", sizes=[(n, n) for n in (16, 32, 48)])

    # Rich light + dark showcase sheet to inspect crispness at 16, 24, 32, 48, 192 + tray
    preview = Image.new("RGB", (860, 340), "#F4F5F7")
    dark_half = Image.new("RGB", (430, 340), "#14171F")
    preview.paste(dark_half, (430, 0))

    for n, x in [(16, 24), (24, 54), (32, 94), (48, 142), (192, 212)]:
        icon = im.resize((n, n), Image.Resampling.LANCZOS)
        y = (280 - n) // 2
        preview.paste(icon, (x, y), icon)
        preview.paste(icon, (430 + x, y), icon)

    tray_d = render_tray_icon("#111827").resize((24, 24), Image.Resampling.LANCZOS)
    tray_l = render_tray_icon("#F8FAFC").resize((24, 24), Image.Resampling.LANCZOS)
    preview.paste(tray_d, (24, 294), tray_d)
    preview.paste(tray_l, (430 + 24, 294), tray_l)

    preview.save(ROOT / "dist/brand-preview.png")


if __name__ == "__main__":
    main()
