#!/usr/bin/env python3
"""Ersetzt die aktuell im Repo hinterlegten Identitaets-Werte (Fork-Repo-URL, DNS-Zone,
ACME-E-Mail, TSIG-Keyname) in gitops/**/*.yaml und charts/**/*.yaml durch eigene Werte aus
deploy.env (oder gleichnamigen Umgebungsvariablen) - siehe deploy.env.example und README.md
## Betrieb.

Die "alten" Werte stehen nirgends als Literal im Skript - stattdessen nur der YAML-Pfad einer
kanonischen Quelldatei je Key (CANONICAL_SOURCES). Der aktuelle Wert wird daraus zur Laufzeit
ausgelesen; erst danach folgt ein reiner Text-Ersatz (keine YAML-Reparsung beim Schreiben),
damit Kommentare/Formatierung in allen Zieldateien 1:1 erhalten bleiben.
"""

import os
import sys
from pathlib import Path
from typing import Any

import yaml

ROOT = Path(__file__).resolve().parent.parent
CONFIG_FILE = ROOT / "deploy.env"

# key -> (Datei relativ zu ROOT, verschachtelter YAML-Pfad), aus der der AKTUELLE Wert
# ausgelesen wird. Jede Datei hier ist die eine kanonische Stelle fuer diesen Wert - andere
# Dateien, die denselben Wert (auch als Teilstring, z.B. "argocd.<zone>") enthalten, werden
# ueber denselben ausgelesenen String mitgetroffen.
CANONICAL_SOURCES: dict[str, tuple[str, tuple[str, ...]]] = {
    "GITHUB_REPO_SSH_URL": ("gitops/apps/app-of-apps.yaml", ("spec", "source", "repoURL")),
    "DNS_ZONE": ("charts/cert-manager-config/values.yaml", ("rfc2136", "zone")),
    "ACME_EMAIL": ("charts/cert-manager-config/values.yaml", ("email",)),
    "RFC2136_TSIG_KEYNAME": ("charts/cert-manager-config/values.yaml", ("rfc2136", "tsigKeyName")),
}

TARGET_DIRS = ["gitops", "charts"]
TARGET_SUFFIXES = {".yaml", ".yml"}


def read_yaml_path(file_path: Path, keys: tuple[str, ...]) -> str:
    data: Any = yaml.safe_load(file_path.read_text())
    for key in keys:
        data = data[key]
    if not isinstance(data, str):
        raise TypeError(f"{file_path}:{'.'.join(keys)} ist kein String: {data!r}")
    return data


def current_values() -> dict[str, str]:
    values = {}
    for key, (rel_path, yaml_keys) in CANONICAL_SOURCES.items():
        values[key] = read_yaml_path(ROOT / rel_path, yaml_keys)
    return values


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


def resolve_new_values() -> dict[str, str]:
    file_values = load_config_file()
    resolved: dict[str, str] = {}
    missing = []
    for key in CANONICAL_SOURCES:
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
    old_values = current_values()
    new_values = resolve_new_values()

    if old_values == new_values:
        print("Keine Aenderung: neue Werte entsprechen den aktuell im Repo hinterlegten.")
        return

    changed = 0
    for path in target_files():
        original = path.read_text()
        text = original
        for key, old_value in old_values.items():
            text = text.replace(old_value, new_values[key])
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
