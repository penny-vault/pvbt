// Stub bundle -- replaced by Vite output in production.
// Registers a fallback component and mounts the Vue app.
(function() {
  const app = Vue.createApp({
    data() {
      return { reportData: __REPORT_DATA__, componentName: __REPORT_COMPONENT__ };
    },
    template: '<component :is="componentName" :data="reportData" />'
  });

  // Fallback component: renders raw JSON when no real component is registered.
  app.component('Fallback', {
    props: ['data'],
    template: '<pre class="p-4 text-sm">{{ JSON.stringify(data, null, 2) }}</pre>'
  });

  app.mount('#app');
})();
