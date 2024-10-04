# SRT Plugin

SRT Plugin is a plugin for M7S that allows you to push and pull streams using SRT protocol.

## Configuration

```yaml
srt:
  listenaddr: :6000
  passphrase: foobarfoobar
```

## Push Stream

srt://127.0.0.1:6000?streamid=publish:/live/test&passphrase=foobarfoobar

## Pull Stream

srt://127.0.0.1:6000?streamid=subscribe:/live/test&passphrase=foobarfoobar
