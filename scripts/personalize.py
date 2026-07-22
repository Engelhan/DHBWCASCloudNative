#!/usr/bin/env python3
"""Ersetzt die DHBW-cas438007-Beispielwerte in gitops/**/*.yaml und charts/**/*.yaml durch
eigene Werte aus deploy.env (oder gleichnamigen Umgebungsvariablen) - siehe
deploy.env.example und README.md ## Betrieb. Reiner Text-Ersatz (keine YAML-Reparsung), damit
Kommentare/Formatierung 1:1 erhalten bleiben.
"""

import os
import sys
from pathlib import Path

ROOT = Path(__file__).resolve().parent.parent
CONFIG_FILE = ROOT / "deploy.env"

# key -> aktueller Wert im Repo, der ersetzt wird.
REPLACEMENTS = {
    "GITHUB_REPO_SSH_URL": "git@github.com:Engelhan/DHBWCASCloudNative.git",
    "DNS_ZONE": "cas438007-at-cas-dhbw-de.users.dhbw.site",
    "ACME_EMAIL": "cas438007@cas.dhbw.de",
    "RFC2136_TSIG_KEYNAME": "user-key-f28a14246f3de83a8ef98b0e727abd6b96a1f124",
}

TARGET_DIRS = ["gitops", "charts"]
TARGET_SUFFIXES = {".yaml", ".yml"}


def load_config_file() -> dict[str, str]:
    values: dict[str, str] = {}
    if not CONFIG_FILE.exists():
        return values
    for line in CONFIG_FILE.read_text().splitlines():
        line = line.strip()
        if not line or line.startswith("#") or "=" not in line:
            continue
        key, _, value = line.partition("=")
        values[key.strip()] = value.strip().strip('"').strip("'")
    return values


def resolve_values() -> dict[str, str]:
    file_values = load_config_file()
    resolved: dict[str, str] = {}
    missing = []
    for key in REPLACEMENTS:
        value = os.environ.get(key) or file_values.get(key)
        if not value:
            missing.append(key)
        else:
            resolved[key] = value
    if missing:
        print(f"Fehlende Werte: {', '.join(missing)}", file=sys.stderr)
        print(
            f"Entweder {CONFIG_FILE.name} aus deploy.env.example anlegen, oder als "
            "Umgebungsvariable setzen.",
            file=sys.stderr,
        )
        sys.exit(1)
    return resolved


def target_files():
    for dirname in TARGET_DIRS:
        for path in (ROOT / dirname).rglob("*"):
            if path.is_file() and path.suffix in TARGET_SUFFIXES:
                yield path


def main() -> None:
    resolved = resolve_values()
    changed = 0
    for path in target_files():
        original = path.read_text()
        text = original
        for key, old_value in REPLACEMENTS.items():
            text = text.replace(old_value, resolved[key])
        if text != original:
            path.write_text(text)
            changed += 1
            print(f"aktualisiert: {path.relative_to(ROOT)}")
    print(f"\n{changed} Datei(en) geaendert. 'git diff' pruefen, dann committen.")
    print(
        "Nicht vergessen: TSIG-Secret-Wert in infra/ansible/secrets.yml und "
        "OpenStack-Credentials in infra/terraform/terraform.tfvars separat eintragen (siehe "
        "jeweilige .example-Dateien) - die bleiben von diesem Skript unberuehrt."
    )


if __name__ == "__main__":
    main()
