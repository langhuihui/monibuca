let summaryES = null
export default {
    state: {
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
    },
    mutations: {
        updateSummary(state, payload) {
            Object.assign(state, payload)
        },
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
                if (!summary.Rooms) summary.Rooms = []
                summary.Rooms.sort((a, b) => a.StreamPath > b.StreamPath ? 1 : -1)
                commit("updateSummary", summary)
            }
        }, stopFetchSummary() {
            summaryES.close()
        }
    }
}