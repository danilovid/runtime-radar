import { isChildCluster } from './argument/child-cluster';

export const environment = {
    api: '/api/v1/',
    singleTenant: ['signin', 'tokens', 'user', 'role', 'cluster'],
    childCluster: isChildCluster,
    // Where "Report a problem" in the assistant sends the request it helped
    // write. Change it to the address that supports this installation.
    supportEmail: 'support@runtime-radar.io',
    pollingInterval: 60000,
    refreshInterval: 300000,
    production: true
};
