//
var supportedWasm = (() => {
  try {
    if (typeof WebAssembly === "object"
      && typeof WebAssembly.instantiate === "function") {

      const module = new WebAssembly.Module(Uint8Array.of(0x0, 0x61, 0x73, 0x6d, 0x01, 0x00, 0x00, 0x00));

      if (module instanceof WebAssembly.Module)
        return new WebAssembly.Instance(module) instanceof WebAssembly.Instance;
    }
  } catch (e) {
  }
  return false;
})();
// WorkerGlobalScope 接口的importScripts() 方法将一个或多个脚本同步导入到工作者的作用域中。
importScripts('ff_wasm.js')
//
importScripts('webgl.js')

var POST_MESSAGE = {
  print: 'print',
  printErr: 'printErr',
  initAudioPlanar: 'initAudioPlanar',
  playAudio: 'playAudio',
  initSize: 'initSize',
  render: 'render',
  init: 'init',
  getProp: 'getProp'
}


//
function dispatchData(input) {
  // await
  let need = input.next()

  let buffer = null

  return (value) => {

    var data = new Uint8Array(value)

    if (buffer) {
      var combine = new Uint8Array(buffer.length + data.length)
      combine.set(buffer)
      combine.set(data, buffer.length)
      data = combine
      buffer = null
    }
    while (data.length >= need.value) {

      var remain = data.slice(need.value)

      need = input.next(data.slice(0, need.value))

      data = remain
    }
    if (data.length > 0) {
      buffer = data
    }
  }
}

if (!Date.now) Date.now = function () {
  return new Date().getTime();
};

//
Module.print = function (text) {
  postMessage({cmd: POST_MESSAGE.print, text: text})
}

//
Module.printErr = function (text) {
  postMessage({cmd: POST_MESSAGE.printErr, text: text})
}

//
Module.postRun = function () {
  // buffer 数组。
  var buffer = []
  //
  var decoder = {
    // options
    opt: {},
    /**
     * @desc 供c++ 调用的 方法。
     * @param channels AVCodecContext->channels 声道数（只针对音频）
     * @param samplerate AVCodecContext->sample_rate 采样率（只针对音频）
     */
    initAudioPlanar: function (channels, samplerate) {
      // 声道数。
      var buffersA = [];
      for (var i = 0; i < channels; i++) {
        buffersA.push([]);
      }

      //
      postMessage({cmd: POST_MESSAGE.initAudioPlanar, samplerate: samplerate, channels: channels})
      /**
       * 供给c++ 调用的方法。播放视频。
       * @param data AVFrame 格式的数据。
       * @param len bytesCount 长度。
       */
      this.playAudioPlanar = function (data, len) {
        // output array
        var outputArray = [];
        //
        var frameCount = len / 4 / buffersA.length;

        for (var i = 0; i < buffersA.length; i++) {
          var fp = HEAPU32[(data >> 2) + i] >> 2;
          var float32 = HEAPF32.subarray(fp, fp + frameCount);
          var buffer = buffersA[i]
          if (buffer.length) {
            buffer = buffer.pop();
            for (var j = 0; j < buffer.length; j++) {
              buffer[j] = float32[j];
            }
          } else {
            buffer = Float32Array.from(float32);
          }
          outputArray[i] = buffer;
        }

        // 声音。
        postMessage({cmd: POST_MESSAGE.playAudio, buffer: outputArray}, outputArray.map(x => x.buffer))
      }
    },
    /**
     *
     * @returns {Generator<number, void, *>}
     */
    inputFlv: function* () {
      // await 9
      yield 9

      var tmp = new ArrayBuffer(4)

      var tmp8 = new Uint8Array(tmp)

      var tmp32 = new Uint32Array(tmp)
      // true
      while (true) {

        tmp8[3] = 0
        //
        var t = yield 15

        var type = t[4]

        tmp8[0] = t[7]

        tmp8[1] = t[6]

        tmp8[2] = t[5]

        var length = tmp32[0]

        tmp8[0] = t[10]

        tmp8[1] = t[9]

        tmp8[2] = t[8]

        var ts = tmp32[0]
        //
        if (ts === 0xFFFFFF) {
          tmp8[3] = t[11]
          ts = tmp32[0]
        }
        // await len
        var payload = yield length

        switch (type) {
          case 8:
            // buffer
            buffer.push({ts, payload, decoder: audioDecoder, type: 0})
            break
          case 9:
            // buffer
            buffer.push({ts, payload, decoder: videoDecoder, type: payload[0] >> 4})
            break
        }
      }
    },
    /**
     *
     * @param url
     */
    play: function (url) {
      console.log('Jessibuca play', url)

      // get delay method
      this.getDelay = function (timestamp) {
        if (!timestamp) return -1
        // first time stamp
        this.firstTimestamp = timestamp
        // start time stamp
        this.startTimestamp = Date.now()

        this.getDelay = function (timestamp) {
          // (现在时间-开始时间) - (时间-第一次时间);
          // 设置 delay 延迟。
          this.delay = (Date.now() - this.startTimestamp) - (timestamp - this.firstTimestamp)
          // 获取到延迟。
          return this.delay
        }
        return -1
      }

      // vod 是否点播模式。
      var loop = this.opt.vod ? () => {
        //
        if (buffer.length) {
          // 取第一个
          var data = buffer[0]
          // 如果没时间戳
          if (this.getDelay(data.ts) === -1) {
            // 抛弃第一个。
            buffer.shift()
            // 执行decode
            data.decoder.decode(data.payload)
          } else {
            // 如果存在的话。
            while (buffer.length) {
              data = buffer[0]
              // 如果超过了 vide buffer。
              if (this.getDelay(data.ts) > this.videoBuffer) {
                // 扔掉。
                buffer.shift()
                // 执行 decode
                data.decoder.decode(data.payload)

              } else {
                //
                break
              }
            }
          }
        }
      } : () => {
        if (buffer.length) {
          // 如果dropping
          if (this.dropping) {
            // 获取第一个数据
            data = buffer.shift()
            // 类型
            if (data.type == 1) {
              //
              this.dropping = false
              //
              data.decoder.decode(data.payload)
            } else if (data.type == 0) {
              //
              data.decoder.decode(data.payload)
            }
          } else {
            // 获取第一个。
            var data = buffer[0]
            //
            if (this.getDelay(data.ts) === -1) {
              buffer.shift()
              data.decoder.decode(data.payload)
            } else if (this.delay > this.videoBuffer + 1000) {
              // 如果delay 草稿
              this.dropping = true
            } else {
              // 遍历 buffer
              while (buffer.length) {
                //
                data = buffer[0]
                //
                if (this.getDelay(data.ts) > this.videoBuffer) {
                  // 删除掉第一个。
                  buffer.shift()
                  data.decoder.decode(data.payload)
                } else {
                  break
                }
              }
            }
          }
        }
      }
      // 设置interval method;
      this.stopId = setInterval(loop, 10)

      // 如果是http
      if (url.indexOf("http") == 0) {
        // flv 模式
        this.flvMode = true

        var _this = this;

        var controller = new AbortController();
        // fetch api
        fetch(url, {signal: controller.signal}).then(function (res) {

          // reader
          var reader = res.body.getReader();

          var input = _this.inputFlv()

          var dispatch = dispatchData(input)
          // fetch next
          var fetchNext = function () {
            // next
            reader.read().then(({done, value}) => {
              if (done) {
                //
                input.return(null)
              } else {
                //
                dispatch(value)

                fetchNext()
              }
            }).catch((e) => input.throw(e))
          }

          fetchNext()
        }).catch(console.error)
        this._close = function () {
          controller.abort()
        }
      } else {
        // 查看是否flv mode
        this.flvMode = url.indexOf(".flv") != -1
        this.ws = new WebSocket(url)
        this.ws.binaryType = "arraybuffer"
        if (this.flvMode) {
          // dispatch
          var dispatch = dispatchData(this.inputFlv())
          // ommessage
          this.ws.onmessage = evt => dispatch(evt.data)
        } else {
          this.ws.onmessage = evt => {
            // dv
            var dv = new DataView(evt.data)

            switch (dv.getUint8(0)) {
              //
              case 1:
                //
                buffer.push({
                  ts: dv.getUint32(1, false),
                  payload: new Uint8Array(evt.data, 5),
                  decoder: audioDecoder,
                  type: 0
                })
                break
              case 2:
                //
                buffer.push({
                  ts: dv.getUint32(1, false),
                  payload: new Uint8Array(evt.data, 5),
                  decoder: videoDecoder,
                  type: dv.getUint8(5) >> 4
                })
                break
            }
          }
        }
        this._close = function () {
          this.ws.close()
        }
      }
      /**
       * @desc 主要是被c++ 端调用的方法。
       * @param w
       * @param h
       */
      this.setVideoSize = function (w, h) {
        postMessage({cmd: "initSize", w: w, h: h})
        var size = w * h
        var qsize = size >> 2
        if (!this.opt.forceNoOffscreen && typeof OffscreenCanvas != 'undefined') {
          var canvas = new OffscreenCanvas(w, h);
          var gl = canvas.getContext("webgl");
          // canvas.
          var render = createWebGL(gl)
          /**
           * @desc 提供给c++ 调用的方法。
           * @param compositionTime
           * @param y
           * @param u
           * @param v
           */
          this.draw = function (compositionTime, y, u, v) {

            render(w, h, HEAPU8.subarray(y, y + size), HEAPU8.subarray(u, u + qsize), HEAPU8.subarray(v, v + (qsize)))
            // transfer to image bitmap
            let image_bitmap = canvas.transferToImageBitmap();

            //
            postMessage({
              cmd: POST_MESSAGE.render,
              compositionTime: compositionTime,
              bps: this.bps,
              delay: this.delay,
              buffer: image_bitmap
            }, [image_bitmap])
          }
        } else {
          /**
           *
           * @param compositionTime
           * @param y
           * @param u
           * @param v
           */
          this.draw = function (compositionTime, y, u, v) {
            var yuv = [HEAPU8.subarray(y, y + size), HEAPU8.subarray(u, u + qsize), HEAPU8.subarray(v, v + (qsize))];
            var outputArray = yuv.map(buffer => Uint8Array.from(buffer))
            postMessage({
              cmd: POST_MESSAGE.render,
              compositionTime: compositionTime,
              bps: this.bps,
              delay: this.delay,
              output: outputArray
            }, outputArray.map(x => x.buffer))
          }
        }
      }
    },
    /**
     *
     */
    close: function () {
      if (this._close) {
        console.log("jessibuca closed")
        this._close()
        audioDecoder.clear()
        videoDecoder.clear()
        clearInterval(this.stopId)
        this.firstTimestamp = 0
        this.startTimestamp = 0
        delete this.getDelay
      }
    }
  }
  //
  var audioDecoder = new Module.AudioDecoder(decoder)
  //
  var videoDecoder = new Module.VideoDecoder(decoder)

  // 发送init 方法到 render 层。
  postMessage({cmd: POST_MESSAGE.init})

  self.onmessage = function (event) {
    var msg = event.data
    switch (msg.cmd) {
      case "init":
        // 设置opt options 参数。
        decoder.opt = msg.opt
        break
      case "getProp":
        // get prop
        postMessage({cmd: POST_MESSAGE.getProp, value: decoder[msg.prop]})
        break
      case "setProp":
        // 设置 prop
        decoder[msg.prop] = msg.value
        break
      case "play":
        // 调用 decode 的 player
        decoder.play(msg.url)
        break
      case "setVideoBuffer":
        //
        decoder.videoBuffer = (msg.time * 1000) | 0
        break
      case "close":
        decoder.close()
        break
    }
  }
}
