<template>
  <div>
    自动更新
    <i-switch v-model="autoUpdate"></i-switch>
    <div id="mountNode"></div>
  </div>
</template>

<script>
import { mapState } from "vuex";
import G6 from "@antv/g6";
var graph = null;
export default {
  data() {
    return {
      autoUpdate: true
    };
  },
  computed: {
    ...mapState({
      data(state) {
        let d = this.addServer(state.summary);
        d.label = "🏠" + d.label;
        return d;
      }
    })
  },
  methods: {
    addServer(node) {
      let result = {
        id: node.Address,
        label: node.Address,
        description: `cpu:${node.CPUUsage >> 0}% mem:${node.Memory.Usage >>
          0}%`,
        shape: "modelRect",
        logoIcon: {
          show: false
        },
        children: []
      };

      if (node.Rooms) {
        for (let i = 0; i < node.Rooms.length; i++) {
          let room = node.Rooms[i];
          let roomId = room.StreamPath;
          let roomData = {
            id: roomId,
            label: room.StreamPath,
            shape: "rect",
            children: []
          };
          result.children.push(roomData);
          if (room.SubscriberInfo) {
            for (let j = 0; j < room.SubscriberInfo.length; j++) {
              let subId = roomId + room.SubscriberInfo[j].ID;
              roomData.children.push({
                id: subId,
                label: room.SubscriberInfo[j].ID
              });
            }
          }
        }
      }
      if (node.Children) {
        for (let childId in node.Children) {
          result.children.push(this.addServer(node.Children[childId]));
        }
      }
      return result;
    }
  },
  watch: {
    data(v) {
      if (graph && this.autoUpdate) {
        //graph.updateChild(v, "");
        graph.changeData(v); // 加载数据
        graph.fitView();
        //graph.read(v);
      }
    }
  },
  mounted() {
    graph = new G6.TreeGraph({
      linkCenter: true,
      // renderer: "svg",
      container: "mountNode", // 指定挂载容器
      width: 800, // 图的宽度
      height: 500, // 图的高度
      modes: {
        default: ["drag-canvas", "zoom-canvas", "click-select", "drag-node"]
      },
      animate: false,
      layout: {
        // type: "indeted",
        direction: "H"
      }
    });
    //graph.addChild(this.data, "");
    graph.read(this.data); // 加载数据
    graph.fitView();
  }
};
</script>

<style>
</style>