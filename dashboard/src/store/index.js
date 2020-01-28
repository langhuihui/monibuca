import Vue from 'vue'
import Vuex from 'vuex'

Vue.use(Vuex)
let summaryES = null

export default new Vuex.Store({
  state: {
    summary:{
      NetWork:[],
      Rooms:[],
      Memory:{
        Used: 0,
        Usage: 0
      },
      CPUUsage:0,
      HardDisk:{
        Used: 0,
        Usage: 0
      }
    }
  },
  mutations: {
    update(state,payload){
      Object.assign(state,payload)
    }
  },
  actions: {
    fetchSummary({commit}){
      summaryES = new EventSource(
          "//" + location.host + "/api/summary"
      );
      summaryES.onmessage = evt=>{
        if (!evt.data) return
        let summary = JSON.parse(evt.data)
        commit("update",{summary})
      }
    },
    stopFetchSummary(){
      summaryES.close()
    }
  },
  modules: {
  }
})
