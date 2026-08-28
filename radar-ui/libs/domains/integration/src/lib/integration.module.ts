import { EffectsModule } from '@ngrx/effects';
import { NgModule } from '@angular/core';
import { StoreModule } from '@ngrx/store';

import { ApiModule } from '@cs/api';

import { IntegrationAIProviderTypeLocalizationPipe } from './pipes/integration-ai-provider-type.pipe';
import { IntegrationEffectStore } from './stores/integration-effect.store';
import { IntegrationTypeLocalizationPipe } from './pipes/integration-type.pipe';
import { INTEGRATION_DOMAIN_KEY, integrationDomainReducer } from './stores/integration-selector.store';

@NgModule({
    imports: [
        ApiModule,
        StoreModule.forFeature(INTEGRATION_DOMAIN_KEY, integrationDomainReducer),
        EffectsModule.forFeature([IntegrationEffectStore])
    ],
    declarations: [IntegrationAIProviderTypeLocalizationPipe, IntegrationTypeLocalizationPipe],
    exports: [IntegrationAIProviderTypeLocalizationPipe, IntegrationTypeLocalizationPipe]
})
export class IntegrationDomainModule {}
