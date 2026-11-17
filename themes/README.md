# Space themes

Themes control appearance only: colors, corners, outlines, icons, the default cover image, and replacement QML for
the three home-screen areas. Feature changes belong in a Space update, not in a theme. See `internal/space/theme.go`
and `thDefault` / `loadTheme()` in `shell/Space.qml`.

- Built-in themes live in `themes/<id>/` and ship with Space updates.
- A catalog theme is packaged as an app: set `"theme": "theme"` in its manifest (the directory containing the
  theme), omit `entry`, and it will not appear in the app grid. Once installed, it is linked under
  `~/.local/share/rmkit-cn/space/themes/<app-id>`. Select it under **Personalize → Themes**.

## theme.json

```json
{
  "id": "contrast", "name": "High contrast", "version": "1.0.0",
  "min_shell": 3,
  "colors": { "line": "#000000", ... },
  "radius": 4, "border": 2,
  "hero": "hero.jpg",
  "preview": "preview.png",
  "icons": { "gear": "icons/gear.svg" },
  "slots": { "hero": "Hero.qml", "widgets": "Widgets.qml", "apps": "Apps.qml" }
}
```

- `colors` defaults to the built-in value when omitted: `bg` page background; `card` card/tile background;
  `fill` light fill; `press` pressed state; `line` card outline; `lineSoft` separators/progress tracks;
  `lineFaint` subtle separators; `text` primary text; `text2` secondary text; `textBody` store body text;
  `text3` details; `textNote` card notes; `primary` primary-button or banner background; `primaryText` its text.
  Dark themes are not supported on e-ink displays: unspecified text defaults to black and backgrounds must remain light.
- `hero` is the theme's cover image. A user-selected cover takes precedence.
- `icons` replaces kit icons; keys are filenames in `kit/icons/` without the `.svg` suffix.
- `slots` replaces home-screen areas: `hero` is the large top card; `widgets` contains today's plan and now-playing
  cards; `apps` contains the My Apps heading and grid. A replacement QML root Item must expose
  `property var space` (the shell API, including helpers such as `space.th`, `space.homeApps()`, `space.open(app)`,
  `space.dateLine()`, `space.localNow()`, and `space.st(key)`) and set `implicitHeight`. The shell controls width.
- When a theme file changes, the shell reloads it from a new URL; restarting xochitl is not required.
