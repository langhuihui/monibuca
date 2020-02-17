<template>
    <div>
        <PathSelector v-model="instancePath" placeholder="输入实例所在的路径"></PathSelector>
        <i-input style="width: 300px;margin:40px auto" v-model="instanceName" :placeholder="defaultInstanceName" search enter-button="Import" @on-search="doImport">
            <span slot="prepend">实例名称</span>
        </i-input>
    </div>
</template>

<script>
    import PathSelector from "./PathSelector"
    export default {
        name: "ImportInstance",
        components:{
            PathSelector
        },
        data(){
            return {
                instancePath:"",
                instanceName:""
            }
        },
        computed:{
            defaultInstanceName(){
                let path = this.instancePath.replace(/\\/g,"/")
                let s = path.split("/")
                if(path.endsWith("/")) s.pop()
                return s.pop()
            }
        },
        methods:{
            doImport(){
                window.ajax.get("/instance/import?path="+this.instancePath+"&name="+this.instanceName).then(x=>{
                    if(x=="success"){
                        this.$Message.success("导入成功！")
                    }else{
                        this.$Message.error(x)
                    }
                })
            }
        }
    }
</script>

<style scoped>

</style>