#!/usr/bin/env python3
"""Makes KDBX 4 fixtures with pykeepass, an implementation of KDBX that is independent of Passbank.

    make_kdbx4.py <out.kdbx> --password P [--keyfile PATH] [--cipher aes256|chacha20|twofish]
                  [--kdf argon2d|argon2id|aeskdf] [--no-compression] [--content rich.xml]
                  [--minor 0|1] [--public-custom-data]

Without --content the database is the empty one that pykeepass makes. With it the XML replaces the
database's XML (protected values are given as plain text with Protected="True"; pykeepass encrypts
them when it saves) and the attachment of rich.py goes to the inner header. The KDF parameters are
small so that tests run fast: Argon2 with 8 MiB, 2 lanes and 2 passes; AES-KDF with 6000 rounds.
"""
import argparse
import os
import sys

from construct import Container
from lxml import etree
from pykeepass import create_database

ARGON2D = bytes.fromhex("ef636ddf8c29444b91f7a9a403e30a0c")
ARGON2ID = bytes.fromhex("9e298b1956db4773b23dfc3ec6f0a1e6")
AESKDF = bytes.fromhex("c9d9f39a628a4460bf740d08c18a4fea")

ATTACH = b"attachment-bytes-for-the-fixture\n" * 3


def set_kdf(dh, kdf):
    params = dh.kdf_parameters.data.dict
    salt = params["S"].value
    items = []
    if kdf in ("argon2d", "argon2id"):
        items = [
            ("$UUID", 0x42, ARGON2D if kdf == "argon2d" else ARGON2ID),
            ("V", 0x04, 0x13), ("I", 0x05, 2), ("M", 0x05, 8 * 1024 * 1024), ("P", 0x04, 2), ("S", 0x42, salt),
        ]
    else:
        items = [("$UUID", 0x42, AESKDF), ("R", 0x05, 6000), ("S", 0x42, salt)]
    params.clear()
    for i, (key, typ, value) in enumerate(items):
        last = i == len(items) - 1
        # next_byte is the type byte of the next item (a Peek when parsing); 0 ends the dictionary.
        nxt = 0 if last else items[i + 1][1]
        params[key] = Container(type=typ, key=key, value=value, next_byte=nxt)


def main():
    ap = argparse.ArgumentParser()
    ap.add_argument("out")
    ap.add_argument("--password")
    ap.add_argument("--keyfile")
    ap.add_argument("--cipher", default="aes256", choices=["aes256", "chacha20", "twofish"])
    ap.add_argument("--kdf", default="argon2d", choices=["argon2d", "argon2id", "aeskdf"])
    ap.add_argument("--no-compression", action="store_true")
    ap.add_argument("--content")
    ap.add_argument("--minor", type=int, default=0, choices=[0, 1])
    ap.add_argument("--public-custom-data", action="store_true")
    a = ap.parse_args()

    if os.path.exists(a.out):
        os.remove(a.out)
    kp = create_database(a.out, password=a.password, keyfile=a.keyfile)
    dh = kp.kdbx.header.value.dynamic_header
    dh.cipher_id.data = a.cipher
    set_kdf(dh, a.kdf)
    if a.no_compression:
        dh.compression_flags.data = Container(compression=False)
    kp.kdbx.header.value.minor_version = a.minor
    if a.content:
        tree = etree.parse(a.content)
        kp.kdbx.body.payload.xml = tree
        kp.add_binary(ATTACH, protected=True)
    if a.public_custom_data:
        # A VariantDictionary: version 1.0, one UInt32 item "fixture" = 42, then the 0 terminator.
        raw = bytes([0x00, 0x01]) + bytes([0x04]) + (7).to_bytes(4, "little") + b"fixture" + (4).to_bytes(4, "little") + (42).to_bytes(4, "little") + bytes([0x00])
        # The field must come before the end-of-header field, or it is not in the header.
        end = dh.pop("end")
        dh["public_custom_data"] = Container(id="public_custom_data", data=raw)
        dh["end"] = end
    kp.save()


if __name__ == "__main__":
    sys.exit(main())
