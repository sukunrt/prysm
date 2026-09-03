#!/usr/bin/env python3
"""Build everything run-shadow-sim.py needs, from pinned sources.

    ./build.py            build all
    ./build.py geth spamoor   build some

Third-party sources are cloned into deps/ at the pinned ref. Binaries land in
bin/. The genesis image prysm-genesis-gen:local is built the way
kurtosis/build-images.sh builds it: the generator fork branch plus a static
prysmctl from this tree. Both directories are gitignored.

lighthouse is the one external item: the bootnode needs ~/dev/lighthouse built.
"""

import argparse
import os
import shutil
import subprocess
import sys
from pathlib import Path

HERE = Path(__file__).resolve().parent
REPO = HERE.parent
DEPS = HERE / "deps"
BIN = HERE / "bin"

# name -> (url, ref). A ref is a branch, a tag or a commit.
SOURCES = {
    "ethshadow": ("https://github.com/sukunrt/ethshadow.git", "decoupled"),
    "genesis-generator": ("https://github.com/sukunrt/ethereum-genesis-generator.git", "decoupled"),
    # kurtosis/network_params.geth-master.yaml: the newer-geth shakeout pin.
    "go-ethereum": ("https://github.com/ethereum/go-ethereum.git",
                    "fd073543c7044fcfa266551b844ba28ff29b233f"),
    # cmd/bootnode left go-ethereum after 1.13.
    "go-ethereum-bootnode": ("https://github.com/ethereum/go-ethereum.git", "v1.13.15"),
    # kurtosis/network_params.yaml spamoor_params.image.
    "spamoor": ("https://github.com/ethpandaops/spamoor.git", "v1.2.3"),
}


def run(cmd, **kw):
    print("+", " ".join(str(c) for c in cmd), flush=True)
    subprocess.run(cmd, check=True, **kw)


def clone(name):
    url, ref = SOURCES[name]
    d = DEPS / name
    if not (d / ".git").exists():
        d.mkdir(parents=True, exist_ok=True)
        run(["git", "init", "-q", str(d)])
        run(["git", "-C", str(d), "remote", "add", "origin", url])
    run(["git", "-C", str(d), "fetch", "-q", "--depth", "1", "origin", ref])
    run(["git", "-C", str(d), "checkout", "-q", "--detach", "FETCH_HEAD"])
    return d


def go_build(cwd, pkg, out, ldflags=None, static=False):
    cmd = ["go", "build", "-o", str(out)]
    if ldflags:
        cmd += ["-ldflags", ldflags]
    if static:
        cmd += ["-tags", "osusergo,netgo", "-ldflags", '-linkmode external -extldflags "-static"']
    run(cmd + [pkg], cwd=cwd, env={**os.environ, "CGO_ENABLED": "1"})


def build_prysm():
    go_build(REPO, "./cmd/beacon-chain", BIN / "prysm-beacon")
    go_build(REPO, "./cmd/validator", BIN / "prysm-validator")


def build_genesis_image():
    gen = clone("genesis-generator")
    ctx = DEPS / "genesis-image-ctx"
    ctx.mkdir(exist_ok=True)
    go_build(REPO, "./cmd/prysmctl", ctx / "prysmctl", static=True)
    run(["docker", "build", "-q", "-t", "decoupled-genesis-gen-base", str(gen)])
    run(["docker", "build", "-q", "-f", str(REPO / "kurtosis/Dockerfile.genesis-gen"),
         "-t", "prysm-genesis-gen:local", str(ctx)])


def build_geth():
    go_build(clone("go-ethereum"), "./cmd/geth", BIN / "geth")


def build_bootnode():
    # 1.13's memsize uses a linkname Go 1.23+ rejects by default.
    go_build(clone("go-ethereum-bootnode"), "./cmd/bootnode", BIN / "bootnode",
             ldflags="-checklinkname=0")


def build_spamoor():
    go_build(clone("spamoor"), "./cmd/spamoor", BIN / "spamoor")


def build_ethshadow():
    d = clone("ethshadow")
    run(["cargo", "build", "-q", "--release"], cwd=d)
    shutil.copy2(d / "target/release/ethshadow", BIN / "ethshadow")


TARGETS = {
    "prysm": build_prysm,
    "genesis-image": build_genesis_image,
    "geth": build_geth,
    "bootnode": build_bootnode,
    "spamoor": build_spamoor,
    "ethshadow": build_ethshadow,
}


def main():
    ap = argparse.ArgumentParser(description=__doc__,
                                 formatter_class=argparse.RawDescriptionHelpFormatter)
    ap.add_argument("targets", nargs="*", default=list(TARGETS))
    args = ap.parse_args()
    for t in args.targets:
        if t not in TARGETS:
            sys.exit(f"unknown target {t}; one of {' '.join(TARGETS)}")
    for tool in ("go", "cargo", "docker", "git", "shadow"):
        if shutil.which(tool) is None:
            sys.exit(f"{tool} is not on PATH")
    lighthouse = Path.home() / "dev/lighthouse/target/release"
    for f in ("lighthouse", "lcli"):
        if not (lighthouse / f).exists():
            sys.exit(f"{lighthouse / f} is missing; build lighthouse first")
    BIN.mkdir(exist_ok=True)
    for t in args.targets:
        print(f"=== {t}", flush=True)
        TARGETS[t]()
    print("built:", " ".join(sorted(p.name for p in BIN.iterdir())))


if __name__ == "__main__":
    main()
