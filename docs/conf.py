"""Sphinx configuration for the celloc documentation site.

The site is Markdown (myst-parser) with Mermaid diagrams (sphinxcontrib-mermaid, rendered
client-side). Build: ``sphinx-build -b html -W docs docs/_build/html``.
"""

project = "celloc"
author = "celloc contributors"

extensions = [
    "myst_parser",  # the Markdown pages
    "sphinxcontrib.mermaid",  # ```mermaid fences, client-side mermaid.js (no Java, no server)
]

# Render ```mermaid fences through sphinxcontrib.mermaid; anchors for in-page links.
myst_fence_as_directive = ["mermaid"]
myst_heading_anchors = 3

html_theme = "furo"

# docs/superpowers is local-only (gitignored) design material, never part of the site.
exclude_patterns = ["_build", "superpowers/**"]
