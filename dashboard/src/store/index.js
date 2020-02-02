import Vue from 'vue'
import Vuex from 'vuex'

Vue.use(Vuex)
let summaryES = null
let logsES = null
export default new Vuex.Store({
  state: {
    summary: {
      Address: location.hostname,
      NetWork: [],
      Rooms: [],
      Memory: {
        Used: 0,
        Usage: 0
      },
      CPUUsage: 0,
      HardDisk: {
        Used: 0,
        Usage: 0
      },
      Children: {}
    }, logs: []
  },
  mutations: {
    update(state, payload) {
      Object.assign(state, payload)
    },
    addLog(state, payload) {
      state.logs.push(payload)
    }
  },
  actions: {
    fetchSummary({ commit }) {
      summaryES = new EventSource(
        "//" + location.host + "/api/summary"
      );
      summaryES.onmessage = evt => {
        if (!evt.data) return
        let summary = JSON.parse(evt.data)
        summary.Address = location.hostname
        commit("update", { summary })
      }
    },
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
    stopFetchSummary() {
      summaryES.close()
    }
  },
  modules: {
  }
})
