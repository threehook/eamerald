# Eamerald backup

mrld-backup creates backup of the eamerald directory store.


## List plugins

```
mrld-backup
Usage: mrld-backup <command>

eamerald backup utility

Commands:
  boltdb    boltdb plugin

Flags:
  -h, --help    Show context-sensitive help.

Run "mrld-backup <command> --help" for more information on a command.

mrld-backup: error: expected "boltdb"
```

NOTES:
 
* Currently only the `boltdb` plugin is exposed

## List input argument of a plugin

```
mrld-backup boltdb

Usage: mrld-backup boltdb --db-file=STRING --backup-dir=STRING

boltdb plugin

Flags:
  -h, --help                 Show context-sensitive help.

      --db-file=STRING       database file path
      --backup-dir=STRING    backup directory path

mrld-backup: error: missing flags: --backup-dir=STRING, --db-file=STRING
```

NOTES:

* In order to list the input arguments for a provider, the plugin name must be provided.

## Execute backup

```
mrld-backup boltdb \
--db-file ~/.local/share/eamerald/db/gdrive-v33.db \
--backup-dir ~/.local/share/eamerald/backup

/Users/<username>/.local/share/eamerald/backup/gdrive-v33-20250731T162842.db
```

NOTES:

* When using the `boltdb` plugin, the mrld-backup command `MUST` be executed on the same machine as were the `mrldd` process is running. As the `mrldd` process holds the exclusive `read-write` connection to the boltdb database file, the backup process uses a `read-only` connection, to copy the content to a new backup file, and flushed the file state to disk when finished.
