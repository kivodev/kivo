#!/usr/bin/env python3
"""Merges a baseline or startup profile into another, in place.

    merge-profile.py INTO FROM

INTO keeps its rules in their order, each with the flags (H, S, P) either
file gives it, and FROM's rules INTO lacks come after, by class then member.
:benchmark records the TV's profiles and the phone's separately; this puts
the phone's into the TV's (see benchmark/build.gradle.kts).
"""
import re
import sys

RULE = re.compile(r"^([HSP]*)(L.+)$")


def read(path):
    rules = {}
    for line in open(path):
        line = line.strip()
        if line and not line.startswith("#"):
            flags, rule = RULE.match(line).groups()
            rules[rule] = rules.get(rule, "") + flags
    return rules


def flags(*sets):
    return "".join(f for f in "HSP" if any(f in s for s in sets))


into, source = sys.argv[1], sys.argv[2]
old, new = read(into), read(source)
with open(into, "w") as out:
    for rule in old:
        out.write(flags(old[rule], new.get(rule, "")) + rule + "\n")
    for rule in sorted(set(new) - set(old), key=lambda r: r.partition("->")[::2]):
        out.write(flags(new[rule]) + rule + "\n")
print(f"{into}: {len(old)} rules, {len(set(new) - set(old))} added")
