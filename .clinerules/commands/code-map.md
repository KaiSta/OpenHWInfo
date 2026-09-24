Load the OpenHWInfo code map before reading or editing source. Read
`.claude/skills/code-map/SKILL.md` in full; use
`.claude/skills/code-map/docs/symbol-index.md` for a symbol's current line and
`.claude/skills/code-map/docs/test-index.md` for a package's fixture helpers when
you are about to read a file or write a test. Then locate the target with a
search and read only a ~40-line window around the hit — never read `main.go`,
`main_test.go` or `website/app.js` whole.