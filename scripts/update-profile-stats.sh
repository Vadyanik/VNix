#!/usr/bin/env bash
set -euo pipefail

# Source an optional settings file; values are inherited by the Python updater.
if [[ $# -gt 0 ]]; then
  set -a
  source "$1"
  set +a
fi

[[ ${PROFILE_ENABLED:-1} == 1 ]] || exit 0
profile_repo=${PROFILE_REPO_PATH:?Set PROFILE_REPO_PATH}
profile_branch=${PROFILE_BRANCH:-main}
profile_remote=${PROFILE_REMOTE:-origin}
profile_readme=${PROFILE_README:-README.md}
export PROFILE_BIRTH_DATE=${PROFILE_BIRTH_DATE:-2026-02-13}
export LC_ALL=C

[[ -d "$profile_repo" ]] || { echo "Profile repository not found: $profile_repo" >&2; exit 1; }
[[ -z $(git -C "$profile_repo" status --porcelain) ]] || {
  echo "Profile repository has pending changes; refusing to overwrite or commit them." >&2
  exit 1
}
[[ $(git -C "$profile_repo" branch --show-current) == "$profile_branch" ]] || {
  echo "Profile repository must be on branch $profile_branch." >&2
  exit 1
}
if [[ ${PROFILE_PULL:-1} == 1 ]]; then
  git -C "$profile_repo" pull --ff-only "$profile_remote" "$profile_branch"
fi

if [[ ${PROFILE_UPDATE_BADGES:-1} == 1 ]]; then
  profile_message=$(python3 - "$profile_repo/$profile_readme" <<'PY'
import datetime
import os
import pathlib
import re
import tempfile
import urllib.parse
import decimal

path = pathlib.Path(__import__('sys').argv[1])
text = path.read_text()
match = re.search(r'^!\[Rebuilds\].*System%20Rebuilds-(\d+)-', text, re.M)
if not match:
    raise SystemExit('Rebuild counter badge not found in profile README')
count = int(match[1]) + 1
now = datetime.datetime.now().astimezone()
start = datetime.date.fromisoformat(os.environ['PROFILE_BIRTH_DATE'])
days = max(1, (now.date() - start).days)
average = (decimal.Decimal(count) / decimal.Decimal(days)).quantize(decimal.Decimal('0.01'), rounding=decimal.ROUND_DOWN)
stamp = urllib.parse.quote(now.strftime('%d.%m.%Y %H:%M'), safe='.:')
badges = {
    'Rebuilds': f'System%20Rebuilds-{count}-blue',
    'Rebuilds Per Day': f'Avg%20Rebuilds%2FDay-{average}-orange',
    'Last Rebuild': f'Last%20Update-{stamp}-blue',
}
for label, badge in badges.items():
    replacement = f'![{label}](https://img.shields.io/badge/{badge}?style=flat-square' + ('&logo=nixos)' if label == 'Rebuilds' else ')')
    text, replaced = re.subn(r'^!\[' + re.escape(label) + r'\].*$', lambda _: replacement, text, flags=re.M)
    if replaced != 1:
        raise SystemExit(f'Expected exactly one {label} badge; found {replaced}')
fd, temporary = tempfile.mkstemp(dir=path.parent, prefix='.vnix-profile-')
try:
    with os.fdopen(fd, 'w') as output:
        output.write(text)
    os.chmod(temporary, path.stat().st_mode & 0o777)
    os.replace(temporary, path)
finally:
    if os.path.exists(temporary):
        os.unlink(temporary)
print(f'profile: rebuild #{count} ({average}/day)')
PY
  )
  if [[ ${PROFILE_COMMIT:-1} == 1 ]]; then
    git -C "$profile_repo" add -- "$profile_readme"
    git -C "$profile_repo" commit -m "$profile_message"
  fi
fi

if [[ ${PROFILE_PUSH:-1} == 1 && ${PROFILE_COMMIT:-1} == 1 ]]; then
  git -C "$profile_repo" push "$profile_remote" "$profile_branch"
fi
