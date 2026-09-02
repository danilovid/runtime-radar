import { CommonModule } from '@angular/common';
import { NgModule } from '@angular/core';
import { FormsModule, ReactiveFormsModule } from '@angular/forms';

import { I18nModule } from '@cs/i18n';
import { McpKeyDomainModule } from '@cs/domains/mcp-key';
import { SharedModule } from '@cs/shared';

import { McpKeyFeatureExpirationColorDirective } from './directives/mcp-key-expiration-color.directive';
import { McpKeyFeatureExpirationLabelPipe } from './pipes/mcp-key-expiration-label.pipe';
import { McpKeyFeatureListContainer } from './containers/list/mcp-key-list.container';
import { McpKeyFeaturePermissionTypePipe } from './pipes/mcp-key-permission-type.pipe';
import { McpKeyFeatureRoutingModule } from './mcp-key-routing.module';
import { McpKeyFeatureSidepanelFormComponent } from './components/sidepanel-form/mcp-key-sidepanel-form.component';

@NgModule({
    imports: [
        CommonModule,
        FormsModule,
        I18nModule,
        ReactiveFormsModule,
        McpKeyDomainModule,
        McpKeyFeatureRoutingModule,
        SharedModule
    ],
    declarations: [
        McpKeyFeatureListContainer,
        McpKeyFeatureSidepanelFormComponent,
        McpKeyFeatureExpirationColorDirective,
        McpKeyFeatureExpirationLabelPipe,
        McpKeyFeaturePermissionTypePipe
    ]
})
export class McpKeyFeatureModule {}
