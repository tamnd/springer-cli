---
title: "Installation"
description: "Install spr from a release, with go install, or from source, and verify what you downloaded."
weight: 20
---

`spr` is one pure Go binary with nothing beside it. Whichever of these you pick, you end up with a single file.

## Prebuilt binaries

Every [release](https://github.com/tamnd/springer-cli/releases) carries archives for Linux, macOS, Windows and FreeBSD. Linux covers amd64, arm64, armv7 and 386; macOS, Windows and FreeBSD cover amd64 and arm64.

```bash
curl -LO https://github.com/tamnd/springer-cli/releases/latest/download/spr_0.2.0_darwin_arm64.tar.gz
tar xzf spr_0.2.0_darwin_arm64.tar.gz
sudo mv spr /usr/local/bin/
```

Windows archives are `.zip` and everything else is `.tar.gz`. If you are on an older release than the one named here, the version in the filename is the one you are downloading.

## Linux packages

deb, rpm and apk are built for the same architectures as the archives, and put the binary at `/usr/bin/spr`.

```bash
sudo dpkg -i spr_0.2.0_amd64.deb          # Debian, Ubuntu
sudo rpm -i spr-0.2.0-1.x86_64.rpm        # Fedora, RHEL, openSUSE
sudo apk add --allow-untrusted spr_0.2.0_x86_64.apk    # Alpine
```

## With Go

```bash
go install github.com/tamnd/springer-cli/cmd/spr@latest
```

That puts `spr` in `$(go env GOPATH)/bin`, which is `~/go/bin` unless you moved it. Make sure that directory is on your `PATH`.

## Container image

```bash
docker run --rm ghcr.io/tamnd/spr:latest --help
```

The image is multi-arch for linux/amd64 and linux/arm64, and its manifest is signed with keyless cosign.

## From source

```bash
git clone https://github.com/tamnd/springer-cli
cd springer-cli
make build        # produces ./bin/spr
./bin/spr version
```

## Verifying what you downloaded

Every release ships a `checksums.txt` covering all of the archives and packages, and that file is signed with keyless [cosign](https://docs.sigstore.dev/), so one signature check covers everything.

```bash
sha256sum --check --ignore-missing checksums.txt

cosign verify-blob checksums.txt \
  --signature checksums.txt.sig \
  --certificate checksums.txt.pem \
  --certificate-identity "https://github.com/tamnd/springer-cli/.github/workflows/release.yml@refs/tags/v0.2.0" \
  --certificate-oidc-issuer "https://token.actions.githubusercontent.com"
```

Keyless means there is no public key to fetch and trust. The certificate records which workflow, in which repository, at which tag, produced the file, and that is what the two `--certificate-` flags are asserting. Change the tag in the identity to match the release you are checking.

Each archive also has an `.sbom.json` beside it listing everything compiled in, which for this tool is three direct dependencies and their transitive set.

## Upgrading from v0.1.0

v0.1.0 shipped a binary called `springer` and this one is called `spr`. They do not overwrite each other, so remove the old one rather than leaving both on your `PATH`. Nothing about the old command line carries over; see the [release notes](/release-notes/).

## Checking the install

```bash
spr version
```

prints the version, the commit it was built from, the Go version and platform, and the settings that are not configurable.
