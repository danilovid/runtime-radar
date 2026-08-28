import { EffectsModule } from '@ngrx/effects';
import { NgModule } from '@angular/core';
import { StoreModule } from '@ngrx/store';

import { ApiModule } from '@cs/api';

import { McpKeyEffectStore } from './stores/mcp-key-effect.store';
import { MCP_KEY_DOMAIN_KEY, mcpKeyDomainReducer } from './stores/mcp-key-selector.store';

@NgModule({
    imports: [
        ApiModule,
        StoreModule.forFeature(MCP_KEY_DOMAIN_KEY, mcpKeyDomainReducer),
        EffectsModule.forFeature([McpKeyEffectStore])
    ]
})
export class McpKeyDomainModule {}
