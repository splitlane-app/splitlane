#!/usr/bin/env bash
# SplitLane: compares what is pinned with what upstream has released and opens one issue per finding, unless an issue
# with the same title exists (open or closed). Checks: a newer sing-box release; a newer Go patch of the pinned minor,
# or the pinned minor out of support; the pinned Electron major out of support (Electron supports its newest three).
# Run weekly by .github/workflows/upstream.yml; needs gh, node and curl, and GH_TOKEN for gh.
# Usage: bash scripts/upstream-check.sh [--dry-run]   (--dry-run prints the findings and opens nothing)
set -euo pipefail
dry=0
[[ ${1:-} == --dry-run ]] && dry=1
cd "$(dirname "$0")/.."

newer() { # newer A B: B is a later version than A
	[[ $1 != "$2" && $(printf '%s\n' "$1" "$2" | sort -V | tail -n 1) == "$2" ]]
}

titles=''
if ((!dry)); then titles=$(gh issue list --state all --limit 500 --json title --jq '.[].title'); fi

report() { # report TITLE BODY
	if grep -Fxq -- "$1" <<<"$titles"; then
		echo "already reported: $1"
	elif ((dry)); then
		echo "would open: $1"
	else
		gh issue create --title "$1" --body "$2" >/dev/null
		echo "opened: $1"
	fi
}

# sing-box: the version the service is built for
pinned=$(sed -n 's/^const CoreVersion = "\(.*\)"$/\1/p' helper/internal/singbox/driver.go)
latest=$(gh api repos/SagerNet/sing-box/releases/latest --jq .tag_name)
latest=${latest#v}
echo "sing-box: pinned $pinned, latest $latest"
[[ -n $pinned && -n $latest ]] || { echo "could not read the sing-box versions" >&2; exit 1; }
if newer "$pinned" "$latest"; then
	if [[ ${pinned%.*} == "${latest%.*}" ]]; then kind='a patch release of the pinned minor'; else kind='a new minor: its config format may differ'; fi
	report "sing-box $latest is out (pinned: $pinned)" "sing-box $latest is out; SplitLane pins $pinned. This is $kind.

Changelog: https://sing-box.sagernet.org/changelog/
Release: https://github.com/SagerNet/sing-box/releases/tag/v$latest

What to do: docs/maintenance.md, \"sing-box\"."
fi

# Go: the toolchain CI builds the service with
pinned=$(sed -n "s/^ *go-version: '\(.*\)'$/\1/p" .github/workflows/build.yml)
minor=${pinned%.*}
stable=$(curl -fsSL 'https://go.dev/dl/?mode=json' | node -e '
let s = ""
process.stdin.on("data", (d) => (s += d)).on("end", () => {
	for (const r of JSON.parse(s)) if (r.stable) console.log(r.version)
})')
latest=$(grep -m 1 "^go$minor\." <<<"$stable" || true)
latest=${latest#go}
echo "Go: pinned $pinned, latest of $minor: ${latest:-none (out of support)}"
[[ -n $pinned ]] || { echo "could not read the Go version of build.yml" >&2; exit 1; }
if [[ -z $latest ]]; then
	report "Go $minor is out of support (pinned: $pinned)" "Go $minor no longer gets security fixes. Supported now: $(tr '\n' ' ' <<<"$stable")

What to do: docs/maintenance.md, \"Go\"."
elif newer "$pinned" "$latest"; then
	report "Go $latest is out (pinned: $pinned)" "Go $latest is out; CI builds the service with $pinned. Go patch releases carry security fixes of the standard library.

Release notes: https://go.dev/doc/devel/release#go$minor.minor

What to do: docs/maintenance.md, \"Go\"."
fi

# Electron: supported are the newest three stable majors
pinned=$(node -p 'require("./app/package.json").devDependencies.electron')
latest=$(curl -fsSL https://registry.npmjs.org/electron/latest | node -p 'JSON.parse(require("fs").readFileSync(0, "utf8")).version')
echo "Electron: pinned $pinned, latest $latest"
if ((${latest%%.*} - ${pinned%%.*} >= 3)); then
	report "Electron ${pinned%%.*} is out of support (latest: $latest)" "Electron supports its newest three stable majors; the latest is $latest and SplitLane pins $pinned, so it gets no more security fixes.

Schedule: https://releases.electronjs.org/schedule

What to do: docs/maintenance.md, \"Electron\"."
fi
