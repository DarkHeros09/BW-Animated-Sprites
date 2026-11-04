"""Build the launcher's plain-zip artifact from an assembled mod directory.

Vendored into this repo so a release can be built from the checked-out tree
alone.  `.github/workflows/release.yml` is the caller: it assembles the
code-only mod with

    go run ./cmd/bwsprites mod -external -out "$RUNNER_TEMP/BW-Animated-Sprites"

and points this script at that directory.  The zip's wrapper folder is the
directory name, which is why the workflow assembles into `BW-Animated-Sprites`:
the launcher installs under the manifest's id, so the wrapper only has to be
tidy, but matching it keeps the archive honest.

`.modkitignore` rules are modkit's own:

  * entries are EXACT relative paths, matched against
    `os.path.relpath(full, mod_root)` -- never a prefix, never a glob.  A
    root-prefixed comparison (or a `startswith`) silently ships dev files.
  * dotted files and dotted directories are skipped at every level, which is
    what keeps `.gitignore`, `.modkitignore` and `.git/` out.
  * entries are stored under `<dir-name>/...` and stamped with a fixed date, so
    two runs of the same tree produce byte-identical archives.

Usage:  python build_release.py <mod-dir> [-o out.zip]
"""

import argparse
import os
import sys
import zipfile

SKIP_DIRS = {".git"}


def ignored_paths(mod_root):
    """The exact relative paths listed in .modkitignore."""
    ignored = set()
    path = os.path.join(mod_root, ".modkitignore")
    if not os.path.isfile(path):
        return ignored
    with open(path, encoding="utf-8") as handle:
        for line in handle:
            line = line.strip()
            if line and not line.startswith("#"):
                ignored.add(line.replace("\\", "/"))
    return ignored


def payload(mod_root, ignored):
    """(relative path, absolute path) for every file the package carries."""
    files = []
    for base, dirs, names in os.walk(mod_root):
        dirs[:] = sorted(d for d in dirs
                         if not d.startswith(".") and d not in SKIP_DIRS)
        for name in sorted(names):
            if name.startswith("."):
                continue
            full = os.path.join(base, name)
            rel = os.path.relpath(full, mod_root).replace(os.sep, "/")
            if rel in ignored:
                continue
            files.append((rel, full))
    return sorted(files)


def main(argv):
    parser = argparse.ArgumentParser(description=__doc__)
    parser.add_argument("mod")
    parser.add_argument("-o", "--output")
    args = parser.parse_args(argv)

    mod_root = os.path.abspath(args.mod)
    mod_id = os.path.basename(mod_root)
    out = args.output or f"{mod_id}.zip"

    ignored = ignored_paths(mod_root)
    files = payload(mod_root, ignored)
    if not files:
        print("nothing to package", file=sys.stderr)
        return 1

    with zipfile.ZipFile(out, "w", zipfile.ZIP_DEFLATED) as archive:
        for rel, full in files:
            info = zipfile.ZipInfo(f"{mod_id}/{rel}",
                                   date_time=(1980, 1, 1, 0, 0, 0))
            info.compress_type = zipfile.ZIP_DEFLATED
            info.external_attr = 0o644 << 16
            with open(full, "rb") as handle:
                archive.writestr(info, handle.read())
    print(f"wrote {out} ({len(files)} files)")
    for rel, _ in files:
        print(f"  {rel}")
    return 0


if __name__ == "__main__":
    sys.exit(main(sys.argv[1:]))
