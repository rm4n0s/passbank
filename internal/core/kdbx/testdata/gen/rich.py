#!/usr/bin/env python3
"""Writes the XML of the rich fixture: a KeePass 2 database that uses most of what KDBX can hold.

    rich.py [--protect memory|protected] [--binaries meta|inner] [--four-one] [--unknown]

make-fixtures.sh gives the XML to `keepassxc-cli import` (--protect memory --binaries meta, which
makes a KDBX 3.1 file) and to make_kdbx4.py (--protect protected --binaries inner). This file is
the single description of the fixture's content: tests spot-check the values below, and
FIXTURES.md lists them. Every password here is a throwaway test value.

--protect memory marks a protected value ProtectInMemory, as a plain KeePass export does: KeePassXC
decrypts a Protected="True" value with the inner stream, which an XML import does not have, and
crashes. --protect protected writes Protected="True" with the plain value, which pykeepass encrypts
when it saves. --binaries meta puts the attachment in Meta/Binaries (KDBX 3); inner leaves it to
the writer (KDBX 4 keeps attachments in the inner header).
--four-one adds the elements of KDBX 4.1; --unknown adds elements that no KeePass version has.
"""
import argparse
import base64
import sys
import uuid

args = None  # set in main


def u(n):
    """The base64 UUID the XML uses, from a small number, so that a UUID is easy to read."""
    return base64.b64encode(uuid.UUID(int=n).bytes).decode()


def t(sec):
    return f"2026-01-0{1 + sec // 86400}T00:00:{sec % 60:02d}Z"


def s(key, value, protected=False):
    flag = ""
    if protected:
        flag = ' ProtectInMemory="True"' if args.protect == "memory" else ' Protected="True"'
    return f"<String><Key>{key}</Key><Value{flag}>{value}</Value></String>"


ATTACH = b"attachment-bytes-for-the-fixture\n" * 3
ICON = base64.b64encode(b"\x89PNG\r\n\x1a\nnot-really-a-png").decode()


def times(c, m, a, expires=False):
    return (f"<Times><CreationTime>{t(c)}</CreationTime><LastModificationTime>{t(m)}</LastModificationTime>"
            f"<LastAccessTime>{t(a)}</LastAccessTime><ExpiryTime>{t(a + 1)}</ExpiryTime>"
            f"<Expires>{'True' if expires else 'False'}</Expires><UsageCount>3</UsageCount>"
            f"<LocationChanged>{t(c)}</LocationChanged></Times>")


def plugin(name):
    """An element no KeePass version defines. Its Token has a Protected attribute but is not a Value
    element: KDBX protects only the text of Value elements, so Token stays plain text and a reader
    must not try to decrypt it."""
    if not args.unknown:
        return ""
    return (f'<X-Plugin-{name} version="2"><Token Protected="True">plugin-secret-{name}</Token>'
            f"<Visible>yes</Visible><Nested><Deep>text</Deep></Nested></X-Plugin-{name}>")


def entry(n, title, user, password, extra="", history="", four_one=""):
    return f"""<Entry><UUID>{u(n)}</UUID><IconID>{n % 60}</IconID>
{times(1, 2, 3)}
{s("Title", title)}{s("UserName", user)}{s("Password", password, True)}{s("URL", "https://example.test/" + title.lower().replace(" ", "-"))}{s("Notes", "notes of " + title)}
{extra}
<AutoType><Enabled>True</Enabled><DataTransferObfuscation>0</DataTransferObfuscation><DefaultSequence>{{USERNAME}}{{TAB}}{{PASSWORD}}{{ENTER}}</DefaultSequence></AutoType>
{four_one}{history}</Entry>"""


def main():
    global args
    ap = argparse.ArgumentParser()
    ap.add_argument("--protect", choices=["memory", "protected"], default="memory")
    ap.add_argument("--binaries", choices=["meta", "inner"], default="meta")
    ap.add_argument("--four-one", action="store_true")
    ap.add_argument("--unknown", action="store_true")
    args = ap.parse_args()

    old_version = entry(0x201, "Bank", "ada", "old-bank-password")
    old_version2 = old_version.replace("old-bank-password", "older-bank-password")
    q41 = f"<QualityCheck>False</QualityCheck><PreviousParentGroup>{u(0x102)}</PreviousParentGroup>" if args.four_one else ""
    bank = entry(
        0x201, "Bank", "ada", "bank-password-1",
        extra=s("PIN", "4711", True) + s("Account number", "DE00 1234")
              + '<Binary><Key>statement.txt</Key><Value Ref="0"/></Binary>'
              + f"<ForegroundColor>#112233</ForegroundColor><BackgroundColor>#445566</BackgroundColor>"
              + f"<OverrideURL>firefox %u</OverrideURL><Tags>money;work</Tags>" + f"<CustomIconUUID>{u(0x301)}</CustomIconUUID>",
        history=f"<History>{old_version2}{old_version}</History>",
        four_one=q41 + plugin("entry"),
    )
    mail = entry(0x202, "Mail", "ada@example.test", "mail-password-1")
    forum = entry(0x203, "Old forum", "ada", "forum-password-1")
    unicode_entry = entry(0x204, "Ünïcödé ключ 鍵", "用户", "pässwörd-日本語")

    binaries = ""
    if args.binaries == "meta":
        binaries = f'<Binaries><Binary ID="0" Compressed="False">{base64.b64encode(ATTACH).decode()}</Binary></Binaries>'
    icon41 = f"<Name>fixture icon</Name><LastModificationTime>{t(8)}</LastModificationTime>" if args.four_one else ""
    settings41 = f"<SettingsChanged>{t(9)}</SettingsChanged>" if args.four_one else ""
    custom41 = f"<LastModificationTime>{t(10)}</LastModificationTime>" if args.four_one else ""
    group41 = f"<Tags>team;home</Tags><PreviousParentGroup>{u(0x103)}</PreviousParentGroup>" if args.four_one else ""
    top_plugin = ("<X-TopLevel>kept<Child a=\"1\"/></X-TopLevel>" if args.unknown else "")

    xml = f"""<?xml version="1.0" encoding="utf-8" standalone="yes"?>
<KeePassFile>
<Meta>
<Generator>passbank fixture generator</Generator>
<DatabaseName>Rich fixture</DatabaseName><DatabaseNameChanged>{t(1)}</DatabaseNameChanged>
<DatabaseDescription>A database that uses most features</DatabaseDescription><DatabaseDescriptionChanged>{t(2)}</DatabaseDescriptionChanged>
<DefaultUserName>ada</DefaultUserName><DefaultUserNameChanged>{t(3)}</DefaultUserNameChanged>
<MaintenanceHistoryDays>365</MaintenanceHistoryDays>
<Color>#336699</Color>
<MasterKeyChanged>{t(4)}</MasterKeyChanged><MasterKeyChangeRec>-1</MasterKeyChangeRec><MasterKeyChangeForce>-1</MasterKeyChangeForce>
<MemoryProtection><ProtectTitle>False</ProtectTitle><ProtectUserName>False</ProtectUserName><ProtectPassword>True</ProtectPassword><ProtectURL>False</ProtectURL><ProtectNotes>False</ProtectNotes></MemoryProtection>
<CustomIcons><Icon><UUID>{u(0x301)}</UUID><Data>{ICON}</Data>{icon41}</Icon></CustomIcons>
<RecycleBinEnabled>True</RecycleBinEnabled><RecycleBinUUID>{u(0x103)}</RecycleBinUUID><RecycleBinChanged>{t(5)}</RecycleBinChanged>
<EntryTemplatesGroup>{u(0)}</EntryTemplatesGroup><EntryTemplatesGroupChanged>{t(6)}</EntryTemplatesGroupChanged>
<HistoryMaxItems>10</HistoryMaxItems><HistoryMaxSize>6291456</HistoryMaxSize>
<LastSelectedGroup>{u(0x102)}</LastSelectedGroup><LastTopVisibleGroup>{u(0x101)}</LastTopVisibleGroup>
{settings41}{binaries}
<CustomData><Item><Key>fixture-key</Key><Value>fixture-value</Value>{custom41}</Item></CustomData>
{plugin("meta")}
</Meta>
<Root>
<Group><UUID>{u(0x101)}</UUID><Name>Root</Name><Notes>root notes</Notes><IconID>48</IconID>
{times(1, 2, 3)}
<IsExpanded>True</IsExpanded><DefaultAutoTypeSequence></DefaultAutoTypeSequence><EnableAutoType>null</EnableAutoType><EnableSearching>null</EnableSearching><LastTopVisibleEntry>{u(0x201)}</LastTopVisibleEntry>
{group41}{plugin("group")}
{bank}{unicode_entry}
<Group><UUID>{u(0x102)}</UUID><Name>Email</Name><Notes></Notes><IconID>19</IconID>
{times(1, 2, 3)}
<IsExpanded>False</IsExpanded><DefaultAutoTypeSequence>{{PASSWORD}}</DefaultAutoTypeSequence><EnableAutoType>false</EnableAutoType><EnableSearching>true</EnableSearching><LastTopVisibleEntry>{u(0)}</LastTopVisibleEntry>
{mail}
<Group><UUID>{u(0x104)}</UUID><Name>Nested</Name><Notes></Notes><IconID>48</IconID>
{times(1, 2, 3)}
<IsExpanded>True</IsExpanded><DefaultAutoTypeSequence></DefaultAutoTypeSequence><EnableAutoType>null</EnableAutoType><EnableSearching>null</EnableSearching><LastTopVisibleEntry>{u(0)}</LastTopVisibleEntry>
</Group>
</Group>
<Group><UUID>{u(0x103)}</UUID><Name>Recycle Bin</Name><Notes></Notes><IconID>43</IconID>
{times(1, 2, 3)}
<IsExpanded>True</IsExpanded><DefaultAutoTypeSequence></DefaultAutoTypeSequence><EnableAutoType>false</EnableAutoType><EnableSearching>false</EnableSearching><LastTopVisibleEntry>{u(0)}</LastTopVisibleEntry>
{forum}
</Group>
</Group>
<DeletedObjects><DeletedObject><UUID>{u(0x401)}</UUID><DeletionTime>{t(7)}</DeletionTime></DeletedObject></DeletedObjects>
</Root>
{top_plugin}
</KeePassFile>
"""
    sys.stdout.write(xml)


if __name__ == "__main__":
    main()
