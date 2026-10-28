#!/usr/bin/env python3
import argparse
import json
import sys
from pathlib import Path


MARKS = {
    "claude": ("  ╲ │ ╱  ", " ── ✳ ── ", "  ╱ │ ╲  "),
    "codex": (" ╭─────╮ ", " │ ❯ ▂ │ ", " ╰─────╯ "),
    "azure": ("    ▄▄   ", "   ╱██╲  ", "  ╱▄▂▂█╲ "),
}
COLORS = {"claude": 173, "codex": 105, "azure": 39}


def mark(name):
    lines = MARKS[name]
    if sys.stdout.isatty():
        return "\n".join(f"\033[38;5;{COLORS[name]}m{line}\033[0m" for line in lines)
    return "\n".join(lines)


def choose(title, options):
    print(f"\n{title}")
    for index, option in enumerate(options, 1):
        print(f"  {index}. {option}")
    while True:
        value = input("Choose a number (q to cancel): ").strip()
        if value.lower() == "q":
            raise KeyboardInterrupt
        if value.isdigit() and 1 <= int(value) <= len(options):
            return options[int(value) - 1]
        print("Enter one of the listed numbers.")


def main():
    parser = argparse.ArgumentParser(description="Burns provider and account picker preview")
    parser.add_argument("--accounts", type=Path, default=Path("accounts.local.json"))
    parser.add_argument("--icons", action="store_true", help="Show the three Unicode mark proposals")
    args = parser.parse_args()
    if args.icons:
        for name in MARKS:
            print(f"\n{name.title()}\n{mark(name)}")
        return
    try:
        accounts = json.loads(args.accounts.read_text())
        for provider in ("claude", "codex"):
            values = accounts[provider]
            if not isinstance(values, list) or not values or any(not isinstance(v, str) or not v.strip() for v in values):
                raise ValueError(f"{provider} needs a nonempty list of account names")
        if any(v.lower() == "azure" for v in accounts["claude"]):
            raise ValueError("Azure is available only for Codex")
    except (OSError, ValueError, KeyError, TypeError) as error:
        parser.error(f"Cannot load accounts: {error}")
    try:
        print("Burns / New Agent Session / Preview")
        for name in ("claude", "codex"):
            print(f"\n{name.title()}\n{mark(name)}")
        provider = choose("Provider", ["Claude", "Codex"]).lower()
        account = choose("Account", accounts[provider])
        print(mark("azure" if account.lower() == "azure" else provider))
        print(f"\nSelected: {provider.title()} / {account}")
        print("Preview complete. No agent was launched.")
    except (KeyboardInterrupt, EOFError):
        print("\nCancelled.")


if __name__ == "__main__":
    main()
