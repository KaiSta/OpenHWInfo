# OpenHWInfo — start here

`AGENTS.md` (repository root) is authoritative: what this project is, the
invariants it must not break, the build and test gates, how to commit.

- Navigate with the **`code-map`** skill (`.claude/skills/code-map/SKILL.md`,
  `/code-map`) before reading source — `main.go`, `main_test.go` and
  `website/app.js` are 600–930 lines each and are read as 40-line windows around a
  search hit, never whole.
- Staging, committing, the git hooks and documentation changes follow the
  **`git-workflow`** skill. Install the hooks once per clone with `make hooks`;
  `pre-commit` gates `gofmt` and the generated code map, `commit-msg` checks the
  subject line.
- `README.md` (what a user sees) and `CODE_REVIEW.md` (how the code got here) are
  read by section and updated in the same commit as the code.
