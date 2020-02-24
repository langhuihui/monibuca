import Vue from 'vue'
import Vuex from 'vuex'
import Summary from './summary'
Vue.use(Vuex)
let logsES = null
export default new Vuex.Store({
  state: {
    logs: []
  },
  mutations: {
    addLog(state, payload) {
      state.logs.push(payload)
    }
  },
  actions: {
    fetchLogs({ commit }) {
      logsES = new EventSource(
        "//" + location.host + "/api/logs"
      )
      logsES.onmessage = evt => {
        if (!evt.data) return
        commit("addLog", evt.data)
      }
    },
    stopFetchLogs() {
      logsES.close()
    },
  },
  modules: {
    summary:Summary
  }
})
