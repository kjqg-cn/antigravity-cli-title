#!/bin/bash
set -e

CMD='title --prefix "My Bot" --max-length 40 --task-timeout 90 --ready "[R]" --working "Working Hard" --waiting "Wait!" --done "All Done"'
if python3 -c 'import sys' >/dev/null 2>&1; then PY_BIN="python3"; else PY_BIN="python"; fi

eval "$($PY_BIN - "$CMD" << 'EOF'
import sys, shlex
cmd = sys.argv[1]
try:
    tokens = shlex.split(cmd)
except Exception:
    tokens = cmd.split()
i = 0
flags = {
    "--prefix": "EXISTING_PREFIX",
    "--max-length": "EXISTING_MAX",
    "--task-timeout": "EXISTING_TIMEOUT",
    "--ready": "EXISTING_READY",
    "--ready-symbol": "EXISTING_READY",
    "--working": "EXISTING_WORKING",
    "--working-symbol": "EXISTING_WORKING",
    "--waiting": "EXISTING_WAITING",
    "--waiting-symbol": "EXISTING_WAITING",
    "--done": "EXISTING_DONE",
    "--done-symbol": "EXISTING_DONE"
}
while i < len(tokens):
    tok = tokens[i]
    matched = False
    for flag, varname in flags.items():
        if tok == flag and i + 1 < len(tokens):
            val = tokens[i+1]
            print(f"{varname}={shlex.quote(val)}")
            i += 1
            matched = True
            break
        elif tok.startswith(flag + "="):
            val = tok[len(flag)+1:]
            print(f"{varname}={shlex.quote(val)}")
            matched = True
            break
    i += 1
EOF
)"

[ "$EXISTING_PREFIX" = "My Bot" ] || exit 1
[ "$EXISTING_MAX" = "40" ] || exit 1
[ "$EXISTING_TIMEOUT" = "90" ] || exit 1
[ "$EXISTING_READY" = "[R]" ] || exit 1
[ "$EXISTING_WORKING" = "Working Hard" ] || exit 1
[ "$EXISTING_WAITING" = "Wait!" ] || exit 1
[ "$EXISTING_DONE" = "All Done" ] || exit 1
echo "[+] Python parser test passed!"

# Test sed -E fallback
for EXISTING_CMD in \
    'title --prefix "My Bot" --max-length 40 --task-timeout 90 --ready "[R]" --working "Working Hard" --waiting "Wait!" --done "All Done"' \
    'title --prefix '\''My Bot'\'' --max-length 40 --task-timeout 90 --ready '\''[R]'\'' --working '\''Working Hard'\'' --waiting '\''Wait!'\'' --done '\''All Done'\'
do
    EXISTING_PREFIX=$(echo "$EXISTING_CMD" | sed -E -n 's/.*--prefix[ =]+("([^"]*)"|'\''([^'\'']*)'\''|([^ ]+)).*/\2\3\4/p')
    EXISTING_MAX=$(echo "$EXISTING_CMD" | sed -E -n 's/.*--max-length[ =]+([0-9]+).*/\1/p')
    EXISTING_TIMEOUT=$(echo "$EXISTING_CMD" | sed -E -n 's/.*--task-timeout[ =]+([0-9]+).*/\1/p')
    EXISTING_READY=$(echo "$EXISTING_CMD" | sed -E -n 's/.*--ready(-symbol)?[ =]+("([^"]*)"|'\''([^'\'']*)'\''|([^ ]+)).*/\3\4\5/p')
    EXISTING_WORKING=$(echo "$EXISTING_CMD" | sed -E -n 's/.*--working(-symbol)?[ =]+("([^"]*)"|'\''([^'\'']*)'\''|([^ ]+)).*/\3\4\5/p')
    EXISTING_WAITING=$(echo "$EXISTING_CMD" | sed -E -n 's/.*--waiting(-symbol)?[ =]+("([^"]*)"|'\''([^'\'']*)'\''|([^ ]+)).*/\3\4\5/p')
    EXISTING_DONE=$(echo "$EXISTING_CMD" | sed -E -n 's/.*--done(-symbol)?[ =]+("([^"]*)"|'\''([^'\'']*)'\''|([^ ]+)).*/\3\4\5/p')

    [ "$EXISTING_PREFIX" = "My Bot" ] || exit 1
    [ "$EXISTING_MAX" = "40" ] || exit 1
    [ "$EXISTING_TIMEOUT" = "90" ] || exit 1
    [ "$EXISTING_READY" = "[R]" ] || exit 1
    [ "$EXISTING_WORKING" = "Working Hard" ] || exit 1
    [ "$EXISTING_WAITING" = "Wait!" ] || exit 1
    [ "$EXISTING_DONE" = "All Done" ] || exit 1
done
echo "[+] sed fallback test passed!"

# Invalid roots must be rejected before restoring a title or touching backups.
"$PY_BIN" - "$PWD/uninstall.sh" "$BASH" <<'EOF'
import json, os, pathlib, subprocess, sys, tempfile
fixture = pathlib.Path(tempfile.mkdtemp(prefix='agy_posix_review_'))
settings = fixture / '.gemini' / 'antigravity-cli'
settings.mkdir(parents=True)
original = '[{"preserve":"user data"}]'
(settings / 'settings.json').write_text(original, encoding='utf-8')
(settings / 'settings.json.pre-title.bak').write_text(json.dumps({'title': {'command':'original'}}), encoding='utf-8')
env = dict(os.environ, HOME=fixture.as_posix())
result = subprocess.run([sys.argv[2], sys.argv[1]], env=env, capture_output=True, text=True, encoding='utf-8')
assert result.returncode != 0, result.stdout
assert (settings / 'settings.json').read_text(encoding='utf-8') == original
assert (settings / 'settings.json.pre-title.bak').exists()
print('[+] POSIX invalid-root preservation passed; fixtures:', fixture)

custom = json.dumps({'title':{'command':'my-custom-title'},'other':123})
(settings / 'settings.json').write_text(custom,encoding='utf-8-sig')
result = subprocess.run([sys.argv[2],sys.argv[1]],env=env,capture_output=True,text=True,encoding='utf-8')
assert result.returncode==0,result.stderr
assert (settings / 'settings.json').read_text(encoding='utf-8-sig')==custom
assert (settings / 'settings.json.pre-title.bak').exists()
print('[+] POSIX uninstall preserves manually replaced titles and backups.')

# Force only the jq branch. jq answers the two read-only queries in this path;
# rm is an audit stub, so --purge control flow is checked without deleting files.
audit = fixture / 'purge-audit.txt'
target = fixture / '.gemini' / 'antigravity-cli-title'
target.mkdir()
manual_backup = settings / 'settings.json.manual.bak'
manual_backup.write_text('manual',encoding='utf-8')
timestamp_backup = settings / 'settings.json.20261125000000.bak'
timestamp_backup.write_text('timestamp',encoding='utf-8')
harness = r'''
python3() { return 1; }
python() { return 1; }
jq() {
    case "$1:$2" in
        '-e:type == "object"') return 0 ;;
        '-r:'*) printf '%s\n' 'my-custom-title' ;;
        *) echo 'Unexpected jq write/query' >&2; return 1 ;;
    esac
}
rm() { printf '%s\n' "$*" >> "$audit"; }
audit="$2"
source "$1" --purge
'''
before = (settings / 'settings.json').read_bytes()
result = subprocess.run([sys.argv[2],'-c',harness,'review',sys.argv[1],str(audit)],env=env,capture_output=True,text=True,encoding='utf-8')
assert result.returncode==0,result.stdout+'\n'+result.stderr
calls = audit.read_text(encoding='utf-8')
assert '-rf ' in calls and 'antigravity-cli-title' in calls,calls
assert timestamp_backup.name in calls,calls
assert manual_backup.name not in calls and 'pre-title.bak' not in calls,calls
assert (settings / 'settings.json').read_bytes()==before
assert (settings / 'settings.json.pre-title.bak').exists()
print('[+] jq user-title branch reaches purge, preserving settings and unrelated backups (rm audited only).')

# Run actual parameter/merge code with an isolated fake local binary and no cleanup.
install_source = pathlib.Path('install.sh').read_text(encoding='utf-8')
start = install_source.index('command_has_flag() {')
end = install_source.index('\nSETTINGS_DIR=',start)
animation_parser = install_source[start:end]
for command,want in [
    ('title --prefix "x --animate y"', 'false'),
    ('title --working --animate', 'false'),
    ('title --prefix=--animate', 'false'),
    ('title --prefix "x --animate y" --animate', 'true'),
    ('title --animate=true', 'true'),
    ('title --animate --animate=false', 'false'),
    ('title --animate=false --animate', 'true'),
]:
    result=subprocess.run([sys.argv[2],'-c',animation_parser+'\nif command_has_animate "$1"; then printf true; else printf false; fi','review',command],capture_output=True,text=True,encoding='utf-8')
    assert result.returncode==0 and result.stdout==want,(command,result.stdout,result.stderr)
print('[+] Token-aware animation inheritance passed.')
for command,wanted,want in [
    ('title --prefix "show --no-prefix --text"', 'no-prefix', 'false'),
    ('title --prefix "show --no-prefix --text"', 'text', 'false'),
    ('title --working "--no-prefix"', 'no-prefix', 'false'),
    ('title --done "--classic"', 'text', 'false'),
    ('title --prefix=--text', 'text', 'false'),
    ('title --prefix "My Bot" --no-prefix', 'no-prefix', 'true'),
    ('title --prefix "My Bot" --text', 'text', 'true'),
    ('title --classic', 'text', 'true'),
]:
    result=subprocess.run([sys.argv[2],'-c',animation_parser+'\nif command_has_flag "$1" "$2"; then printf true; else printf false; fi','review',command,wanted],capture_output=True,text=True,encoding='utf-8')
    assert result.returncode==0 and result.stdout==want,(command,wanted,result.stdout,result.stderr)
print('[+] Token-aware prefix/text inheritance passed.')
local_repo = fixture / 'repo'
local_repo.mkdir()
fake_bin = local_repo / 'title'
fake_bin.write_text('#!/bin/sh\nexit 0\n',encoding='utf-8')
(local_repo / 'uninstall.sh').write_text('#!/bin/sh\nexit 0\n',encoding='utf-8')
# Local-prebuilt installation calls rm -f on a nonexistent temp path: omit only that
# cleanup in this test to honor the workspace deletion-confirmation policy.
install_source = install_source.replace('        rm -f "$TMP_EXE"','        :')
installer = local_repo / 'install.sh'
installer.write_text(install_source,encoding='utf-8')
initial = {'title':{'command':'/x/antigravity-cli-title/title --prefix "BOM Bot"'},'other':456}
(settings / 'settings.json').write_text(json.dumps(initial),encoding='utf-8-sig')
result=subprocess.run([sys.argv[2],str(installer),'--animate'],env=env,capture_output=True,text=True,encoding='utf-8')
assert result.returncode==0, result.stdout+'\n'+result.stderr
installed=json.loads((settings/'settings.json').read_text(encoding='utf-8-sig'))
assert '--animate' in installed['title']['command'] and 'BOM Bot' in installed['title']['command'] and installed['other']==456
result=subprocess.run([sys.argv[2],str(installer),'--no-animate'],env=env,capture_output=True,text=True,encoding='utf-8')
assert result.returncode==0,result.stdout+'\n'+result.stderr
installed=json.loads((settings/'settings.json').read_text(encoding='utf-8-sig'))
assert '--animate' not in installed['title']['command']
print('[+] POSIX --animate/--no-animate and BOM reinstall passed.')
initial['title']['command'] = '/x/antigravity-cli-title/title --prefix "show --no-prefix --text"'
(settings / 'settings.json').write_text(json.dumps(initial),encoding='utf-8')
result=subprocess.run([sys.argv[2],str(installer)],env=env,capture_output=True,text=True,encoding='utf-8')
assert result.returncode==0,result.stdout+'\n'+result.stderr
installed=json.loads((settings/'settings.json').read_text(encoding='utf-8'))
import shlex
tokens=shlex.split(installed['title']['command'])
assert '--no-prefix' not in tokens and '--text' not in tokens,tokens
assert tokens[tokens.index('--prefix')+1]=='show --no-prefix --text',tokens
print('[+] POSIX reinstall preserves flag-looking prefix text without changing modes.')
EOF
