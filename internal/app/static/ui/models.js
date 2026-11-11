/** @typedef {'system'|'light'|'dark'} Theme */
/** @typedef {{state:string, model:string, backend:string, active:number, requests:number, error?:string, detail?:string}} RuntimeState */
/** @typedef {{id:string, model:string, model_version:string, backend:string, auto_start:boolean, idle_minutes:number, tuning:Record<string,number|string>, status:RuntimeState, model_revision:string, deployment_revision:string}} Deployment */
/** @typedef {{id:string, kind:string, state:string, step:string, progress:number, total:number, trace_id:string, deployment_id?:string, error?:string, detail?:string}} Operation */
/** @typedef {{status:RuntimeState, settings:{theme:Theme, language:string, model:string, backend:string, engine:string}, deployments:Deployment[], operations:Operation[], version:string, platform:string, endpoint:string}} StatusView */
export {};
