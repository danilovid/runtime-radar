import { EffectsModule } from '@ngrx/effects';
import { NgModule } from '@angular/core';
import { StoreModule } from '@ngrx/store';

import { AssistantEffectStore } from './stores/assistant-effect.store';
import { ASSISTANT_DOMAIN_KEY, assistantDomainReducer } from './stores/assistant-selector.store';

@NgModule({
    // No ApiModule here: this domain's services are providedIn: 'root', and the
    // application libraries import each other in cycles, so an eager module
    // that reaches @cs/api first leaves those namespaces uninitialised at
    // bootstrap.
    imports: [
        StoreModule.forFeature(ASSISTANT_DOMAIN_KEY, assistantDomainReducer),
        EffectsModule.forFeature([AssistantEffectStore])
    ]
})
export class AssistantDomainModule {}
