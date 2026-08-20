import { EffectsModule } from '@ngrx/effects';
import { NgModule } from '@angular/core';
import { StoreModule } from '@ngrx/store';

import { ApiModule } from '@cs/api';

import { AssistantEffectStore } from './stores/assistant-effect.store';
import { ASSISTANT_DOMAIN_KEY, assistantDomainReducer } from './stores/assistant-selector.store';

@NgModule({
    imports: [
        ApiModule,
        StoreModule.forFeature(ASSISTANT_DOMAIN_KEY, assistantDomainReducer),
        EffectsModule.forFeature([AssistantEffectStore])
    ]
})
export class AssistantDomainModule {}
