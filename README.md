### covid

Developer tool to clone Red Hat bugzilla bugs in terminal.

NOTE: This is very hacky tool and you need to set `BUGZILLA_EMAIL` and `BUGZILLA_PASSWORD` env vars in order for it to work.

There are no guarantees this won't break, but hey, PR's are welcomed.

```
Usage of ./covid:
  -bug string
        Specify the source bug (eg. '1812863')
  -target string
        Specify the target release (eg. '4.3.z')
```

