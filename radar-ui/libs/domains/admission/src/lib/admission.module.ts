import { EffectsModule } from '@ngrx/effects';
import { NgModule } from '@angular/core';
import { StoreModule } from '@ngrx/store';

import { ApiModule } from '@cs/api';

import { AdmissionEffectStore } from './stores/admission-effect.store';
import { ADMISSION_DOMAIN_KEY, admissionDomainReducer } from './stores/admission-selector.store';

@NgModule({
    imports: [
        ApiModule,
        StoreModule.forFeature(ADMISSION_DOMAIN_KEY, admissionDomainReducer),
        EffectsModule.forFeature([AdmissionEffectStore])
    ]
})
export class AdmissionDomainModule {}
