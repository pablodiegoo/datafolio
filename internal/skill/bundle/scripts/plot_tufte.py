# /// script
# requires-python = ">=3.11"
# dependencies = [
#   "matplotlib>=3.9",
# ]
# ///
"""Optional Tufte-aligned Matplotlib helper for DataFolio Skill Bundles.

This script runs strictly outside the DataFolio Go binary via `uv run` (ADR-0002)
and consumes `datafolio.mplstyle` and `tokens.json` emitted by
`datafolio tokens export --to matplotlib`.
"""

from __future__ import annotations

import argparse
import csv
import json
from pathlib import Path
from typing import Any

import matplotlib as mpl
import matplotlib.pyplot as plt
from cycler import cycler


DEFAULT_COLORS: dict[str, Any] = {
    "paper": "#fffff8",
    "paper_subtle": "#fbf9f2",
    "paper_alt": "#f6f3ea",
    "ink": "#111111",
    "ink_soft": "#2b2926",
    "muted": "#6b6559",
    "accent": "#8b1e1e",
    "ochre": "#854d0e",
    "forest": "#166534",
    "rule": "#ded8ca",
    "rule_strong": "#3d3932",
    "palette": ["#8b1e1e", "#111111", "#854d0e", "#166534", "#6b6559"],
}


def load_tokens(tokens_path: str | Path | None) -> dict[str, Any]:
    """Load and normalize a `tokens.json` file exported by `datafolio tokens export`."""
    if tokens_path is None:
        return {"theme": "paper", "colors": dict(DEFAULT_COLORS), "fonts": {}}
    path = Path(tokens_path)
    with path.open("r", encoding="utf-8") as fh:
        data = json.load(fh)
    colors = dict(DEFAULT_COLORS)
    colors.update(data.get("colors") or {})
    data["colors"] = colors
    return data


def resolve_colors(tokens: dict[str, Any], dark: bool = False) -> dict[str, Any]:
    """Return active color tokens, optionally overlaying `tokens['dark']`."""
    colors = dict(DEFAULT_COLORS)
    colors.update(tokens.get("colors") or {})
    if dark and isinstance(tokens.get("dark"), dict):
        for key, value in tokens["dark"].items():
            if value:
                colors[key] = value
        palette = [
            colors.get(k)
            for k in ("accent", "ink", "ochre", "forest", "muted")
            if colors.get(k)
        ]
        if palette:
            colors["palette"] = palette
    return colors


def apply_tufte_style(
    mplstyle_path: str | Path | None = None,
    tokens: dict[str, Any] | None = None,
    *,
    dark: bool = False,
) -> dict[str, Any]:
    """Apply `datafolio.mplstyle` and `tokens.json` rcParams, returning active colors."""
    if mplstyle_path is not None:
        style_file = Path(mplstyle_path)
        if style_file.exists():
            plt.style.use(str(style_file))

    resolved_tokens = tokens or {"colors": dict(DEFAULT_COLORS), "fonts": {}}
    colors = resolve_colors(resolved_tokens, dark=dark)
    fonts = resolved_tokens.get("fonts") or {}

    palette = colors.get("palette") or [
        colors["accent"],
        colors["ink"],
        colors["ochre"],
        colors["forest"],
        colors["muted"],
    ]

    mpl.rcParams.update(
        {
            "figure.facecolor": colors["paper"],
            "axes.facecolor": colors["paper"],
            "savefig.facecolor": colors["paper"],
            "text.color": colors["ink"],
            "axes.labelcolor": colors["ink_soft"],
            "xtick.color": colors["muted"],
            "ytick.color": colors["muted"],
            "axes.edgecolor": colors["rule_strong"],
            "axes.linewidth": 0.8,
            "axes.spines.top": False,
            "axes.spines.right": False,
            "axes.grid": True,
            "axes.axisbelow": True,
            "grid.color": colors["rule"],
            "grid.linewidth": 0.5,
            "grid.linestyle": ":",
            "axes.prop_cycle": cycler(color=palette),
            "svg.fonttype": "none",
        }
    )

    if fonts.get("serif"):
        mpl.rcParams["font.serif"] = [fonts["serif"], "Georgia", "DejaVu Serif"]
    if fonts.get("sans"):
        mpl.rcParams["font.sans-serif"] = [fonts["sans"], "DejaVu Sans"]
    if fonts.get("mono"):
        mpl.rcParams["font.monospace"] = [fonts["mono"], "DejaVu Sans Mono"]

    return colors


def tufte_axes(
    ax: mpl.axes.Axes,
    colors: dict[str, Any],
    *,
    grid_axis: str = "y",
) -> mpl.axes.Axes:
    """Configure minimal Tufte axes styling without chartjunk."""
    ax.spines["top"].set_visible(False)
    ax.spines["right"].set_visible(False)
    ax.spines["left"].set_color(colors["rule_strong"])
    ax.spines["bottom"].set_color(colors["rule_strong"])
    ax.tick_params(axis="both", which="both", length=3, colors=colors["muted"])
    ax.grid(True, axis=grid_axis, color=colors["rule"], linewidth=0.5, linestyle=":")
    if grid_axis != "both":
        other = "x" if grid_axis == "y" else "y"
        ax.grid(False, axis=other)
    return ax


def save_figure(
    fig: mpl.figure.Figure,
    out_path: str | Path,
    colors: dict[str, Any],
    *,
    dpi: int = 300,
) -> Path:
    """Save figure cleanly as SVG or PNG using the Token Contract paper surface."""
    dest = Path(out_path)
    dest.parent.mkdir(parents=True, exist_ok=True)
    fig.savefig(
        dest,
        dpi=dpi,
        bbox_inches="tight",
        facecolor=colors["paper"],
        edgecolor="none",
    )
    return dest


def _load_csv_series(csv_path: Path) -> tuple[list[float | str], dict[str, list[float]]]:
    with csv_path.open("r", encoding="utf-8", newline="") as fh:
        reader = csv.DictReader(fh)
        if not reader.fieldnames or len(reader.fieldnames) < 2:
            raise ValueError(f"{csv_path}: expected at least 2 columns (x and >=1 series)")
        x_col = reader.fieldnames[0]
        y_cols = list(reader.fieldnames[1:])
        xs: list[float | str] = []
        series: dict[str, list[float]] = {col: [] for col in y_cols}
        for row in reader:
            raw_x = (row[x_col] or "").strip()
            try:
                xs.append(float(raw_x))
            except ValueError:
                xs.append(raw_x)
            for col in y_cols:
                series[col].append(float(row[col]))
        return xs, series


def build_chart(
    *,
    xs: list[Any],
    series: dict[str, list[float]],
    colors: dict[str, Any],
    title: str = "",
    xlabel: str = "",
    ylabel: str = "",
) -> mpl.figure.Figure:
    """Render a clean, direct-labeled multi-series line chart with zero chartjunk."""
    fig, ax = plt.subplots(figsize=(6.8, 3.6))
    tufte_axes(ax, colors, grid_axis="y")

    palette = colors.get("palette") or [colors["accent"], colors["ink"], colors["ochre"], colors["forest"]]
    for idx, (label, ys) in enumerate(series.items()):
        color = palette[idx % len(palette)]
        ax.plot(xs, ys, marker="o", markersize=3.5, linewidth=1.6, color=color, label=label)
        if xs and ys:
            ax.annotate(
                f" {label}",
                xy=(xs[-1], ys[-1]),
                va="center",
                ha="left",
                fontsize=9,
                color=color,
            )

    if title:
        ax.set_title(title, loc="left", fontsize=11, pad=10, color=colors["ink"])
    if xlabel:
        ax.set_xlabel(xlabel, color=colors["ink_soft"])
    if ylabel:
        ax.set_ylabel(ylabel, color=colors["ink_soft"])

    fig.tight_layout()
    return fig


def main(argv: list[str] | None = None) -> int:
    parser = argparse.ArgumentParser(
        description="Render a Tufte-styled SVG or PNG chart using DataFolio Token Contract exports."
    )
    parser.add_argument("--mplstyle", help="Path to exported datafolio.mplstyle")
    parser.add_argument("--tokens", help="Path to exported tokens.json")
    parser.add_argument("--input", help="Optional CSV file (first column x, remaining columns numeric series)")
    parser.add_argument("--out", required=True, help="Output figure path (.svg or .png)")
    parser.add_argument("--title", default="", help="Chart title (plain text, zero emojis)")
    parser.add_argument("--xlabel", default="", help="X-axis label")
    parser.add_argument("--ylabel", default="", help="Y-axis label")
    parser.add_argument("--dpi", type=int, default=300, help="Raster DPI for PNG output")
    parser.add_argument("--dark", action="store_true", help="Apply dark surface tokens if present in tokens.json")
    parser.add_argument("--demo", action="store_true", help="Render an illustrative series when --input is omitted")
    args = parser.parse_args(argv)

    tokens = load_tokens(args.tokens)
    colors = apply_tufte_style(args.mplstyle, tokens, dark=args.dark)

    if args.input:
        xs, series = _load_csv_series(Path(args.input))
    elif args.demo:
        xs = ["Q1", "Q2", "Q3", "Q4"]
        series = {
            "Baseline": [100.0, 108.5, 114.2, 121.0],
            "Target": [100.0, 104.0, 110.0, 116.5],
        }
    else:
        parser.error("provide --input <data.csv> or --demo")
        return 2

    fig = build_chart(
        xs=xs,
        series=series,
        colors=colors,
        title=args.title,
        xlabel=args.xlabel,
        ylabel=args.ylabel,
    )
    save_figure(fig, args.out, colors, dpi=args.dpi)
    plt.close(fig)
    return 0


if __name__ == "__main__":
    raise SystemExit(main())
