---
description: Daily DISCOVER→PLAN→EXECUTE→VERIFY→ITERATE triage of open PRs and issues
---

# Daily PR & Issue Triage Loop

You are running an autonomous daily triage loop on the `1broseidon/ketch` repo.
Your job: process every untriaged open PR and issue, auto-fix what is provably
low-risk, and flag everything else for human approval. Work the loop below
**one item at a time**. Be conservative: when in doubt, NOTIFY rather than fix.

## State memory (idempotency)

The `triaged` label is your memory. An item carrying `triaged` was already
handled on a prior run — skip it unless it has new activity since that label was
applied. Apply `triaged` to every item only AFTER you finish handling it. This
makes the loop safe to run daily without redoing work.

## DISCOVER

```bash
gh pr list   --state open --json number,title,updatedAt,labels --search '-label:triaged'
gh issue list --state open --json number,title,updatedAt,labels --search '-label:triaged'
```

Also re-include any `triaged` item whose latest comment/commit is newer than the
`triaged` label (new human activity = re-triage). If nothing is left to process,
STOP and emit the digest (see below).

## PLAN (per item)

Classify and assign a risk tier. The tier is mechanical, not a judgment call:

**AUTO-FIX** — only if ALL of these hold:
- typo/docs fix, lint/format, a dependency **patch** bump, an added missing
  test, OR a small **localized** bug fix
- the diff is tight and does NOT touch the CLI flag surface, `--json`/frontmatter
  output format, or the config schema (`config/`)
- the change is objectively verifiable by the test suite

**NOTIFY** — anything else, including: changes to CLI flags / output format /
config schema / cache / crawl architecture / security; anything tests don't
cover; anything large, ambiguous, or that you are unsure about.

For open PRs: review the diff. If it's a clean low-risk PR, you may leave an
approving review comment but DO NOT merge. Otherwise NOTIFY.

## EXECUTE

**AUTO-FIX path:**
1. `git checkout -b triage/<issue-number>-<slug>` off `main`.
2. Make the minimal fix.
3. Proceed to VERIFY. Only open a PR if verification is green.

**NOTIFY path:**
1. Do NOT touch code.
2. Write a concise triage summary: what the item is, root cause if known,
   proposed fix, and why it needs your approval (which gate it tripped).

## VERIFY (gate — auto-fix cannot ship without this)

```bash
make build && make test && make lint
```

- All green → open the PR:
  `gh pr create --base main --title "fix: <summary> (closes #<n>)" --body "<what/why + 'make build/test/lint all pass'>"`
- Any failure → ONE retry. If still failing, abandon the branch
  (`git checkout main && git branch -D triage/...`) and DOWNGRADE the item to
  the NOTIFY path. Never open a red PR.

## ITERATE & finalize (per item)

After the item reaches a terminal state (PR opened, or notify summary posted):
1. Post the summary as a GitHub comment on the item: `gh issue comment <n> --body ...`
   (or `gh pr comment`).
2. Label it: AUTO-FIX → add `triaged`. NOTIFY → add `triaged` AND `needs-review`.
3. Move to the next item.

## STOP & digest

When no untriaged items remain, emit ONE rolled-up digest and send a push
notification with the headline counts:
- ✅ Auto-fixed (PR opened): list `#n → PR #m`
- 🔎 Needs your review: list `#n → reason`
- ⏭️ Skipped (already triaged, no new activity): count only

Use the PushNotification tool for the headline (e.g.
"Ketch triage: 1 PR opened, 1 needs review"). Keep the full digest in your final
message. Do not merge anything. Do not push to `main`.
