<template>
  <div id="mountNode"></div>
</template>

<script>
import { mapState } from "vuex";
import G6 from "@antv/g6";
var graph = null;
export default {
  computed: {
    ...mapState({
      data(state) {
        let summary = state.summary;
        // 点集
        let nodes = [];
        // 边集
        let edges = [];
        this.addServer(summary, nodes, edges);
        return {
          nodes,
          edges
        };
      }
    })
  },
  methods: {
    addServer(node, nodes, edges) {
      let result = {
        id: node.Address,
        label: node.Address,
        description: `cpu:${node.CPUUsage >> 0}% mem:${node.Memory.Usage >>
          0}%`,
        shape: "modelRect",
        logoIcon: {
          show: false
        }
      };
      nodes.push(result);
      if (node.Rooms) {
        for (let i = 0; i < node.Rooms.length; i++) {
          let room = node.Rooms[i];
          let roomId = result.id + room.StreamPath;
          nodes.push({
            id: roomId,
            label: room.StreamPath,
            shape: "rect"
          });
          edges.push({ source: result.id, target: roomId });
          if (room.SubscriberInfo) {
            for (let j = 0; j < room.SubscriberInfo.length; j++) {
              let subId = roomId + room.SubscriberInfo[j].ID;
              nodes.push({
                id: subId,
                label: room.SubscriberInfo[j].ID
              });
              edges.push({ source: roomId, target: subId });
            }
          }
        }
      }
      if (node.Children && node.Children.length > 0) {
        for (let i = 0; i < node.Children.length; i++) {
          let child = this.addServer(node.Children[i], nodes, edges);
          edges.push({
            source: result.id,
            target: child.id
          });
        }
      }
      return result;
    }
  },
  watch: {
    data(v) {
      if (graph) {
        graph.read(v); // 加载数据
      }
    }
  },
  mounted() {
    graph = new G6.Graph({
      renderer: "svg",
      container: "mountNode", // 指定挂载容器
      width: 800, // 图的宽度
      height: 500, // 图的高度
      layout: {
        type: "radial"
      },
      defaultNode: {}
    });
    graph.read(this.data); // 加载数据
  }
};
</script>

<style>
</style>