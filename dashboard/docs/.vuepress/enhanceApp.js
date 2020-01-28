import upperFirst from 'lodash/upperFirst'
  import camelCase from 'lodash/camelCase'
  export default ({
    Vue, // the version of Vue being used in the VuePress app
    options, // the options for the root Vue instance
    router, // the router instance for the app
    siteData // site metadata
  }) => {
    // ...apply enhancements to the app
   
    const requireComponent = require.context(
      // The relative path of the components folder
      '../../src/components',
      // Whether or not to look in subfolders
      true,
      // The regular expression used to match base component filenames
      /.(vue|js)$/
    )
    
    
    requireComponent.keys().forEach(fileName => {
      // Get component config
      const componentConfig = requireComponent(fileName)
      const fc = fileName.split('/')
      const f = fc[fc.length - 1]
      // Get PascalCase name of component
      const componentName = upperFirst(
        camelCase(
          f.replace(/.*\//, '$1').replace(/\.\w+$/,'')
        )
      )
      // Register component globally
      Vue.component(
        componentName,
        componentConfig.default || componentConfig
      )
    })
  }