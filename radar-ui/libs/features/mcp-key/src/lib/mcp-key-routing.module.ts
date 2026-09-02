import { NgModule } from '@angular/core';
import { RouterModule, Routes } from '@angular/router';

import { mcpKeyActivateGuard } from '@cs/domains/mcp-key';

import { McpKeyFeatureListContainer } from './containers/list/mcp-key-list.container';

const routes: Routes = [
    {
        path: '',
        component: McpKeyFeatureListContainer,
        canActivate: [mcpKeyActivateGuard],
        data: {
            localizationTitleKey: 'McpKey.ListPage.Header.Title'
        }
    }
];

@NgModule({
    imports: [RouterModule.forChild(routes)],
    exports: [RouterModule]
})
export class McpKeyFeatureRoutingModule {}
