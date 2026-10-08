#!/usr/bin/env python3
"""Back up and quarantine the music library before clearing its database rows."""

from __future__ import annotations

import argparse
import datetime as dt
import os
from pathlib import Path
import sqlite3
import sys
from typing import Sequence


PROJECT_ROOT = Path(__file__).resolve().parent.parent
CONFIRMATION = "DELETE ALL MUSIC"


def resolve_project_path(raw_path: str) -> Path:
    path = Path(raw_path)
    if not path.is_absolute():
        path = PROJECT_ROOT / path
    candidate = Path(os.path.abspath(path))
    if not candidate.is_relative_to(PROJECT_ROOT):
        raise ValueError(f"path must remain inside the project directory: {raw_path}")
    if not candidate.resolve().is_relative_to(PROJECT_ROOT):
        raise ValueError(f"path resolves outside the project directory: {raw_path}")
    return candidate


def list_mp3_files(media_dir: Path) -> list[Path]:
    if media_dir.is_symlink() or not media_dir.is_dir():
        raise ValueError(f"MP3 directory is missing or is a symlink: {media_dir}")

    files: list[Path] = []
    for path in sorted(media_dir.iterdir()):
        if path.suffix.lower() != ".mp3":
            continue
        if path.is_symlink():
            raise ValueError(f"refusing to move symlink instead of an MP3 file: {path}")
        if path.is_file():
            files.append(path)
    return files


def song_count(connection: sqlite3.Connection) -> int:
    table = connection.execute(
        "SELECT 1 FROM sqlite_master WHERE type = 'table' AND name = 'songs'"
    ).fetchone()
    if table is None:
        raise ValueError("music database does not contain the expected songs table")
    columns = {
        row[1] for row in connection.execute("PRAGMA table_info(songs)").fetchall()
    }
    if "file_path" not in columns:
        raise ValueError("songs table is missing the expected file_path column")
    return int(connection.execute("SELECT COUNT(*) FROM songs").fetchone()[0])


def verify_database(connection: sqlite3.Connection) -> None:
    result = connection.execute("PRAGMA integrity_check").fetchone()
    if result is None or result[0] != "ok":
        raise ValueError(f"music database integrity check failed: {result}")
    song_count(connection)


def make_backup(database_path: Path, backup_path: Path) -> None:
    source = sqlite3.connect(database_path)
    destination = sqlite3.connect(backup_path)
    try:
        source.backup(destination)
    finally:
        destination.close()
        source.close()
    os.chmod(backup_path, 0o600)


def restore_quarantined_files(moved_files: list[tuple[Path, Path]]) -> list[str]:
    errors: list[str] = []
    for original, quarantined in reversed(moved_files):
        try:
            if original.exists():
                raise FileExistsError(f"refusing to overwrite {original}")
            quarantined.replace(original)
        except OSError as error:
            errors.append(f"{quarantined} -> {original}: {error}")
    return errors


def clear_library(database_path: Path, media_dir: Path) -> tuple[Path, int, int]:
    if database_path.is_symlink() or media_dir.is_symlink():
        raise ValueError("database and MP3 directory must not be symlinks")
    database_path = database_path.resolve(strict=True)
    media_dir = media_dir.resolve(strict=True)
    if not database_path.is_file():
        raise ValueError(f"music database is missing: {database_path}")

    check_connection = sqlite3.connect(database_path)
    try:
        verify_database(check_connection)
    finally:
        check_connection.close()

    timestamp = dt.datetime.now(dt.timezone.utc).strftime("%Y%m%dT%H%M%S%fZ")
    archive_dir = media_dir.parent / f"music-cleanup-{timestamp}"
    archive_dir.mkdir(mode=0o700)
    quarantine_dir = archive_dir / "mp3s"
    quarantine_dir.mkdir(mode=0o700)
    backup_path = archive_dir / "music.db"

    try:
        make_backup(database_path, backup_path)
    except Exception:
        backup_path.unlink(missing_ok=True)
        quarantine_dir.rmdir()
        archive_dir.rmdir()
        raise

    moved_files: list[tuple[Path, Path]] = []
    connection = sqlite3.connect(database_path, timeout=30)
    try:
        connection.execute("PRAGMA foreign_keys = ON")
        connection.execute("BEGIN IMMEDIATE")
        verify_database(connection)
        current_files = list_mp3_files(media_dir)
        current_songs = song_count(connection)

        for source in current_files:
            destination = quarantine_dir / source.name
            source.replace(destination)
            moved_files.append((source, destination))

        deleted = connection.execute("DELETE FROM songs").rowcount
        if deleted != current_songs:
            raise RuntimeError(
                f"song count changed during cleanup (expected {current_songs}, deleted {deleted})"
            )
        remaining = song_count(connection)
        if remaining != 0:
            raise RuntimeError(f"database still contains {remaining} song rows")

        foreign_key_errors = connection.execute("PRAGMA foreign_key_check").fetchall()
        if foreign_key_errors:
            raise RuntimeError(f"database has foreign-key violations: {foreign_key_errors}")
        connection.commit()
    except Exception:
        connection.rollback()
        restore_errors = restore_quarantined_files(moved_files)
        if restore_errors:
            print(
                "Could not restore all MP3 files; their recoverable copies remain in "
                f"{quarantine_dir}:",
                file=sys.stderr,
            )
            for error in restore_errors:
                print(f"  {error}", file=sys.stderr)
        raise
    finally:
        connection.close()

    return archive_dir, current_songs, len(current_files)


def parse_args(argv: Sequence[str] | None = None) -> argparse.Namespace:
    parser = argparse.ArgumentParser(
        description="Back up the music database, clear all song rows, and quarantine MP3 files."
    )
    parser.add_argument(
        "--db",
        default="internal/database/music.db",
        help="music database path, relative to the project root",
    )
    parser.add_argument(
        "--media-dir",
        default="data/mp3s",
        help="MP3 directory, relative to the project root",
    )
    parser.add_argument(
        "--apply",
        action="store_true",
        help="perform the cleanup after an exact interactive confirmation; otherwise preview only",
    )
    return parser.parse_args(argv)


def main(argv: Sequence[str] | None = None) -> int:
    args = parse_args(argv)
    try:
        database_path = resolve_project_path(args.db)
        media_dir = resolve_project_path(args.media_dir)
        if not database_path.is_file() or database_path.is_symlink():
            raise ValueError(f"music database is missing or is a symlink: {database_path}")
        if not media_dir.is_dir() or media_dir.is_symlink():
            raise ValueError(f"MP3 directory is missing or is a symlink: {media_dir}")

        connection = sqlite3.connect(database_path.as_uri() + "?mode=ro", uri=True)
        try:
            verify_database(connection)
            tracks = song_count(connection)
        finally:
            connection.close()
        files = list_mp3_files(media_dir)

        print(f"Project:       {PROJECT_ROOT}")
        print(f"Database:      {database_path}")
        print(f"MP3 directory: {media_dir}")
        print(f"Song rows:     {tracks}")
        print(f"MP3 files:     {len(files)} (direct children only)")
        if not args.apply:
            print("Preview only; nothing was changed. Re-run with --apply to continue.")
            return 0

        print("Stop the Jaylub service before continuing.")
        print("The database will be backed up and MP3 files moved to a quarantine folder.")
        typed = input(f'Type "{CONFIRMATION}" to proceed: ')
        if typed != CONFIRMATION:
            print("Confirmation did not match; nothing was changed.")
            return 1

        archive_dir, removed_tracks, quarantined_files = clear_library(
            database_path, media_dir
        )
        print("Music library cleared.")
        print(f"Song rows removed: {removed_tracks}")
        print(f"MP3 files quarantined: {quarantined_files}")
        print(f"Database backup and recoverable MP3 files: {archive_dir}")
        print("Keep this archive until you have verified the result.")
        return 0
    except (EOFError, OSError, sqlite3.Error, RuntimeError, ValueError) as error:
        print(f"Music cleanup failed: {error}", file=sys.stderr)
        return 1


if __name__ == "__main__":
    raise SystemExit(main())
