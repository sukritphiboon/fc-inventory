"""
Version utilities.

Pure helpers for parsing/comparing semantic versions, plus a thin wrapper to
query the latest published GitHub release. Kept dependency-light and free of
side effects so it can be unit-tested without network access.
"""

import re

import requests

# Public GitHub repository used for the "new version available" notice.
GITHUB_REPO = "sukritphiboon/fc-inventory"


def parse_version(value):
    """
    Parse a version string into a comparable (major, minor, patch) tuple.

    Accepts a leading 'v' and ignores any pre-release/build suffix:
        'v1.2.3'      -> (1, 2, 3)
        '1.1'         -> (1, 1, 0)
        '2.0.0-rc.1'  -> (2, 0, 0)
    """
    if not value:
        return (0, 0, 0)
    core = re.split(r"[-+]", value.strip().lstrip("vV"))[0]
    nums = []
    for part in core.split("."):
        match = re.match(r"\d+", part)
        nums.append(int(match.group()) if match else 0)
    while len(nums) < 3:
        nums.append(0)
    return tuple(nums[:3])


def is_newer(latest, current):
    """Return True if `latest` is a strictly newer version than `current`."""
    return parse_version(latest) > parse_version(current)


def get_latest_release(repo=GITHUB_REPO, timeout=4):
    """
    Fetch the latest published release for `repo` from the GitHub API.

    Returns a dict with 'tag' and 'url'. Raises on network/HTTP errors so the
    caller can decide how to degrade gracefully.
    """
    url = f"https://api.github.com/repos/{repo}/releases/latest"
    resp = requests.get(
        url,
        timeout=timeout,
        headers={"Accept": "application/vnd.github+json"},
    )
    resp.raise_for_status()
    data = resp.json()
    return {
        "tag": data.get("tag_name", "") or "",
        "url": data.get("html_url", "") or "",
    }
