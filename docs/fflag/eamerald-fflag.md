# Eamerald Feature Flags

## Feature Flag

The `EAMERALD_FFLAG` environment variable contains a bitmask value, in the form of a unsigned 64 bit integer value, which is used to enable new features that are in development in an isolated manner.

Current feature flags:

* `EAMERALD_FFLAG=1` enable the editor options

To set the feature flags for the terminal session:

```console
export EAMERALD_FFLAG=1
```

The feature flag value can also be passed in an ad-hoc manner like:

```
EAMERALD_FFLAG=1 eamerald directory get object --help
```

Note that the help output will now contain the `edit request` flag:

```
 -e, --edit                     edit request
```

which indicates that the editor feature flag is activated.
