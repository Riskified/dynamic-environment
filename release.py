#!/usr/bin/env python3

import subprocess

DEFAULT_NOTES = 'release-notes.md.tmpl'


class SemanticVersion:
    """
    A class that handles semantic version. Just instantiate with valid string version.
    """

    def __init__(self, major: int, minor: int, patch: int):
        self.major = major
        self.minor = minor
        self.patch = patch

    def __str__(self):
        return f"v{self.major}.{self.minor}.{self.patch}"

    def bump_major(self):
        self.major += 1

    def bump_minor(self):
        self.minor += 1

    def bump_patch(self):
        self.patch += 1

    @classmethod
    def from_string(cls, version: str):
        try:
            major, minor, patch = map(int, version.lstrip('v').split('.', 2))
            return cls(major, minor, patch)
        except Exception as err:
            raise Exception(f"Error parsing '{version}' as semantic version: {err}")


def extract_tag():
    tmp = subprocess.run(["git", "describe", "--tags", "--exact"], capture_output=True)
    if tmp.returncode == 0:
        raise Exception(f"There is a tag pointing to current commit: {tmp.stdout.decode().rstrip()}")
    latest_tag = subprocess.run(["git", "describe", "--tags", "--abbrev=0"], capture_output=True)
    return latest_tag.stdout.rstrip().decode()


def mk_release(ver: SemanticVersion, notes_file: str, dry_run: bool):
    notes_params = ['-F', notes_file] if notes_file else ['-F', DEFAULT_NOTES]

    cmd = ["gh", "release", 'create', '-n', ''] + notes_params + ['-t', f"Release: {ver}", f"{ver}"]
    if dry_run:
        print(f"DRY RUN: would create release with version: {ver}")
        print(f"DRY RUN: would run: {cmd}")
    else:
        subprocess.run(cmd)
        print("")
        print('A new release + tag were created on github. Please make sure to pull the changes locally '
              'and verify that docker image is building.')
        if not notes_file:
            print('')
            print('No notes file provided. Opening release in web browser to edit...')
            input('Print ENTER to exit ')
            subprocess.run(['gh', 'release', 'view', '-w', str(ver)])


def run(args):
    tag = extract_tag()
    ver = SemanticVersion.from_string(tag)
    match args.level:
        case 'major':
            ver.bump_major()
        case 'minor':
            ver.bump_minor()
        case 'patch':
            ver.bump_patch()
        case _:
            raise Exception("No level supplied")

    mk_release(ver, args.notes, args.dry)


if __name__ == "__main__":
    import sys
    import argparse

    parser = argparse.ArgumentParser(
        prog="Release",
        description='Bump version (tag) according to \'level\' and create github release. '
                    f"Copy and edit '{DEFAULT_NOTES}' to add release notes, otherwise we will open a browser to "
                    'manually edit.',
        epilog="IMPORTANT: This script assumes you have both 'git' and 'gh' "
               "configured and running correctly on your machine",
    )
    parser.add_argument('level', choices=['major', 'minor', 'patch'])
    parser.add_argument(
        '--dry-run',
        help='Do not create release + tag on github, Just print the version that would have been created',
        action=argparse.BooleanOptionalAction,
        dest='dry',
        default=False
    )
    parser.add_argument(
        '-F', '--notes-file',
        help='Markdown file containing release notes',
        dest='notes'
    )
    args = parser.parse_args()
    try:
        run(args)
    except Exception as err:
        print(err, file=sys.stderr)
        sys.exit(1)
