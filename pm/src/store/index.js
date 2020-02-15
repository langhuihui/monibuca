import Vue from 'vue'
import Vuex from 'vuex'
Vue.use(Vuex)

export default new Vuex.Store({
  state: {
    defaultPlugins:{
      GateWay:[
          "gateway",'ListenAddr = ":8081"',"网关插件，提供各种API服务，包括信息采集和控制等，控制台页面展示（静态资源服务器）"
      ],
      LogRotate:[
          "logrotate",`Path = "log"
Size = 0
Days = 1`,"日志分割插件，Size 代表按照字节数分割，0代表采用时间分割"
      ],
      Jessica:[
          "jessica",'ListenAddr = ":8080"',"WebSocket协议订阅，采用私有协议，搭配Jessibuca播放器实现低延时播放"
      ],
      Cluster:[
          "cluster",'Master = "localhost:2019"\nListenAddr = ":2019"',"集群插件，可以实现级联转发功能，Master代表上游服务器，ListenAdder代表源服务器监听端口，可只配置一项"
      ],
      RTMP:[
          "rtmp",'ListenAddr = ":1935"',"rtmp协议实现，基本发布和订阅功能"
      ],
      RecordFlv:[
          "record",'Path="./resource"',"录制视频流到flv文件"
      ],
      HDL:[
          "HDL",'ListenAddr = ":2020"',"Http-flv格式实现，可以对接CDN厂商进行回源拉流"
      ],
      Auth:[
          "auth",'Key = "www.monibuca.com"',"一个鉴权验证模块"
      ],
      Qos:[
          "QoS",'Suffix = ["high","medium","low"]',"质量控制插件，可以动态改变订阅的不同的质量的流"
      ]
    }
  },
  mutations: {
  },
  actions: {
  },
  modules: {
  }
})
