"""Take render-blocking CSS/JS out of the critical rendering path.

Lighthouse flagged main/palette CSS, extra.css and the theme bundle as
blocking first paint. This hook rewrites the built HTML so that:

- the theme stylesheets (main + palette) and extra.css are inlined into
  one <style> element, concatenated in the original cascade order;
- relative font URLs in extra.css are rewritten per page so they keep
  resolving from the page's location;
- the theme JS bundle gets the defer attribute.

Inlining (rather than async-loading) avoids a flash of unstyled content
and the layout shift it causes; the stylesheets are small (~160 KB raw,
~27 KB gzipped) and contain no relative url() references besides the
extra.css fonts handled above.
"""

import re
from pathlib import Path

_MAIN_RE = re.compile(
    r'\s*<link rel="stylesheet" href="[^"]*assets/stylesheets/main\.[^"]+\.min\.css">'
)
_PALETTE_RE = re.compile(
    r'\s*<link rel="stylesheet" href="[^"]*assets/stylesheets/palette\.[^"]+\.min\.css">'
)
_EXTRA_RE = re.compile(r'\s*<link rel="stylesheet" href="([^"]*stylesheets/)extra\.css">')
_BUNDLE_RE = re.compile(
    r'<script src="([^"]*assets/javascripts/bundle\.[^"]+\.min\.js)"></script>'
)

_css_cache = {}


def _read_theme_css(site_dir, stem):
    if stem not in _css_cache:
        matches = sorted(Path(site_dir).glob(f"assets/stylesheets/{stem}.*.min.css"))
        _css_cache[stem] = matches[0].read_text(encoding="utf-8") if matches else None
    return _css_cache[stem]


def _transform(output, config):
    main_match = _MAIN_RE.search(output)
    if not main_match:
        return output

    main_css = _read_theme_css(config["site_dir"], "main")
    palette_css = _read_theme_css(config["site_dir"], "palette")
    if main_css is None:
        return output

    extra_match = _EXTRA_RE.search(output)
    parts = [main_css]
    if palette_css is not None:
        parts.append(palette_css)
    if extra_match:
        extra_css = Path(config["docs_dir"]) / "stylesheets" / "extra.css"
        css = extra_css.read_text(encoding="utf-8")
        # Keep font URLs working from the page's location.
        parts.append(css.replace('url("fonts/', f'url("{extra_match.group(1)}fonts/'))

    style = "<style>" + "\n".join(parts) + "</style>"
    output = output[: main_match.start()] + style + output[main_match.end():]
    output = _PALETTE_RE.sub("", output, count=1)
    output = _EXTRA_RE.sub("", output, count=1)
    return _BUNDLE_RE.sub(r'<script src="\1" defer></script>', output)


def on_post_page(output, page, config):
    return _transform(output, config)


def on_post_build(config):
    # 404.html and other extra templates are not run through on_post_page.
    extra_template = Path(config["site_dir"]) / "404.html"
    if extra_template.is_file():
        html = extra_template.read_text(encoding="utf-8")
        extra_template.write_text(_transform(html, config), encoding="utf-8")
