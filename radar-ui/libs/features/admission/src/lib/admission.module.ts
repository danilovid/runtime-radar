import { CommonModule } from '@angular/common';
import { NgModule } from '@angular/core';
import { FormsModule, ReactiveFormsModule } from '@angular/forms';

import { AdmissionDomainModule } from '@cs/domains/admission';
import { I18nModule } from '@cs/i18n';
import { SharedModule } from '@cs/shared';

import { AdmissionFeatureDetailsContainer } from './containers/details/admission-details.container';
import { AdmissionFeatureEventsContainer } from './containers/events/admission-events.container';
import { AdmissionFeatureFilterPopoverComponent } from './components/filter-popover/admission-filter-popover.component';
import { AdmissionFeatureRoutingModule } from './admission-routing.module';
import { AdmissionFeatureSettingsContainer } from './containers/settings/admission-settings.container';
import { AdmissionFeatureSidepanelPolicyComponent } from './components/sidepanel-policy/admission-sidepanel-policy.component';
import { AdmissionFeatureSidepanelPolicyFormComponent } from './components/sidepanel-policy-form/admission-sidepanel-policy-form.component';

@NgModule({
    imports: [
        CommonModule,
        FormsModule,
        I18nModule,
        AdmissionDomainModule,
        AdmissionFeatureRoutingModule,
        ReactiveFormsModule,
        SharedModule
    ],
    declarations: [
        AdmissionFeatureDetailsContainer,
        AdmissionFeatureEventsContainer,
        AdmissionFeatureFilterPopoverComponent,
        AdmissionFeatureSettingsContainer,
        AdmissionFeatureSidepanelPolicyComponent,
        AdmissionFeatureSidepanelPolicyFormComponent
    ]
})
export class AdmissionFeatureModule {}
