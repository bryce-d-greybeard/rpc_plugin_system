#!/usr/bin/env bash
set -euo pipefail

repo_root="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
cd "${repo_root}"

if [[ -e src/workflow.toml ]]; then
  echo "workflow.toml must live at repo root, not under src/" >&2
  exit 1
fi

if find src \( -name WORKFLOW.md -o -path '*/evidence/*' -o -path '*/review/*' -o -path '*/postmortems/*' -o -path 'src/artifacts/*' \) | grep -q .; then
  echo "workflow records must not live under src/" >&2
  find src \( -name WORKFLOW.md -o -path '*/evidence/*' -o -path '*/review/*' -o -path '*/postmortems/*' -o -path 'src/artifacts/*' \) >&2
  exit 1
fi

python3 - <<'PY'
from pathlib import Path
import sys, tomllib

manifest = Path('workflow.toml')
if not manifest.is_file():
    sys.exit('missing repo-root workflow.toml')

data = tomllib.loads(manifest.read_text())
coverage = data.get('coverage', {})
if coverage.get('map') != 'workflow/artifacts/global-coverage-map.md':
    sys.exit('coverage.map must point at workflow/artifacts/global-coverage-map.md')

for root in data.get('root_features', []):
    rid = root['id']
    path = root.get('path', '')
    source = root.get('source_path', '')
    if not path.startswith('workflow/root/'):
        sys.exit(f'root {rid}: path must live under workflow/root/')
    if not source.startswith('src/'):
        sys.exit(f'root {rid}: source_path must live under src/')
    if path == source:
        sys.exit(f'root {rid}: path and source_path must be separated')
    if not Path(path, 'WORKFLOW.md').is_file():
        sys.exit(f'root {rid}: missing workflow record at {path}/WORKFLOW.md')
    if not Path(source).is_dir():
        sys.exit(f'root {rid}: missing source dir {source}')

for feat in data.get('features', []):
    fid = feat['id']
    path = feat.get('path', '')
    source = feat.get('source_path', '')
    if not path.startswith('workflow/features/'):
        sys.exit(f'feature {fid}: path must live under workflow/features/')
    if not source.startswith('src/'):
        sys.exit(f'feature {fid}: source_path must live under src/')
    if path == source:
        sys.exit(f'feature {fid}: path and source_path must be separated')
    p = Path(path)
    if not (p / 'WORKFLOW.md').is_file():
        sys.exit(f'feature {fid}: missing workflow record at {path}/WORKFLOW.md')
    for sub in ('evidence', 'review', 'postmortems'):
        if not (p / sub).is_dir():
            sys.exit(f'feature {fid}: missing workflow/{sub} directory')
    if not Path(source).is_dir():
        sys.exit(f'feature {fid}: missing source dir {source}')
PY
