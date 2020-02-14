<template>
    <div>
        <i-input v-model="instanceName" :placeholder="defaultInstanceName"></i-input>
        <i-input prefix="ios-home" v-model="instancePath" placeholder="输入实例所在的路径" search enter-button="Import" @on-search="doImport">
        </i-input>
    </div>
</template>

<script>
    export default {
        name: "ImportInstance",
        data(){
            return {
                instancePath:"",
                instanceName:""
            }
        },
        computed:{
            defaultInstanceName(){
                return this.instancePath.replace(/\\/g,"/").split("/").pop()
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