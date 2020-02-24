<template>
  <Modal v-bind="$attrs" draggable v-on="$listeners" title="查看订阅者">
    <Table :columns="subtableColumns" :data="data"></Table>
  </Modal>
</template>

<script>
import StartTime from "./StartTime"
export default {
  props: {
    data: Array
  },
  data() {
    return {
      subtableColumns: [
        {
          title: "类型",
          key: "Type"
        },
        {
          title: "Name",
          key: "ID"
        },
        {
          title: "订阅时间",
          render(h, { row }) {
            return h(StartTime, {
              props: {
                value: row.SubscribeTime
              }
            });
          }
        },
        {
          title: "丢帧",
          render(h, { row }) {
            return h(
              "span",
              row.TotalPacket ? row.TotalDrop + "/" + row.TotalPacket : ""
            );
          }
        },
        {
          title: "Buffer",
          render(h, { row }) {
            return h("Progress", {
              props: {
                percent: Math.floor((row.BufferLength * 99) / 1024),
                "text-inside": true,
                "stroke-width": 20,
                "stroke-color": ["#87d068", "#ff0000"]
              }
            });
          }
        }
      ]
    };
  }
};
</script>

<style>
</style>