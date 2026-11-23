# Fonts

The Jennifer brand's two typefaces, the same files the website serves
(`website/public/fonts/`), for the docs theme in `../jennifer.toml`. Grimoire
copies them into the built site's `assets/fonts/`, so the docs make no
third-party request.

| File | Family | Axes | Used for |
| ---- | ------ | ---- | -------- |
| `archivo-variable.woff2` | [Archivo](https://fonts.google.com/specimen/Archivo) | `wdth` 62-125, `wght` 100-900 | headlines (expanded) and running text |
| `splinesansmono-variable.woff2` | [Spline Sans Mono](https://fonts.google.com/specimen/Spline+Sans+Mono) | `wght` 300-700 | code |

Both are licensed under the [SIL Open Font License 1.1](https://openfontlicense.org/),
which permits redistribution. They are unmodified upstream files, Latin subset.
