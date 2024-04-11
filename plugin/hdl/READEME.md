# HDL Plugin

The main function of the HDL plugin is to provide access to the HTTP-FLV protocol.

HTTP-FLV protocol (HDL: Http Dynamic Live) is a dynamic streaming media live broadcast protocol, which implements the function of live broadcast of FLV format video on the ordinary HTTP protocol. The meaning of its name can be mainly divided into three parts:

- HTTP (HyperText Transfer Protocol): Hypertext Transfer Protocol, a protocol for information transfer on the World Wide Web. In the HTTP-FLV protocol, HTTP serves as the basic protocol to provide the basic structure of data transmission.

- FLV (Flash Video): A streaming media video format, initially designed by Adobe, mainly used for online playback of short videos or live broadcasts.

- HDL: Http Dynamic Live, is the abbreviation of HTTP-FLV protocol alias, which can be understood as "HTTP-based dynamic live broadcast protocol", emphasizing that it is based on the original HTTP protocol, through dynamic technology to achieve the function of video live broadcast.

## Plugin Address

https://github.com/Monibuca/plugin-hdl

## Plugin Introduction
```go
import (
    _ "m7s.live/plugin/hdl/v4"
)
```

## Default Plugin Configuration

```yaml
hdl:
  pull: # Format: https://m7s.live/guide/config.html#%E6%8F%92%E4%BB%B6%E9%85%8D%E7%BD%AE
```

## Plugin Features

### Pulling HTTP-FLV Streams from M7S

If the live/test stream already exists in M7S, then HTTP-FLV protocol can be used for playback. If the listening port is not configured, then the global HTTP port is used (default 8080).

```bash
ffplay http://localhost:8080/hdl/live/test.flv
```

### M7S Pull HTTP-FLV Streams from Remote

The available API is:
`/hdl/api/pull?target=[HTTP-FLV address]&streamPath=[stream identifier]&save=[0|1|2]`
- save meaning: 0 - do not save 1 - save to pullonstart 2 - save to pullonsub
- HTTP-FLV address needs to be urlencoded to prevent special characters from affecting parsing
