import { NgModule } from '@angular/core';
import { RouterModule, Routes } from '@angular/router';

import { PermissionName, rolePermissionsResolver } from '@cs/domains/role';
import {
    admissionActivateGuard,
    admissionConfigModifyDeactivateGuard,
    admissionDeactivateGuard
} from '@cs/domains/admission';

import { AdmissionFeatureDetailsContainer } from './containers/details/admission-details.container';
import { AdmissionFeatureEventsContainer } from './containers/events/admission-events.container';
import { AdmissionFeatureSettingsContainer } from './containers/settings/admission-settings.container';
import { AdmissionRouterName } from './interfaces/admission-navigation.interface';

const routes: Routes = [
    {
        path: '',
        canActivate: [admissionActivateGuard],
        canDeactivate: [admissionDeactivateGuard],
        children: [
            {
                path: AdmissionRouterName.SETTINGS,
                component: AdmissionFeatureSettingsContainer,
                canDeactivate: [admissionConfigModifyDeactivateGuard],
                resolve: {
                    permissions: rolePermissionsResolver
                },
                data: {
                    localizationTitleKey: 'Admission.SettingsPage.Header.Title',
                    permissions: [PermissionName.SYSTEM, PermissionName.RULES]
                }
            },
            {
                path: AdmissionRouterName.EVENTS,
                component: AdmissionFeatureEventsContainer,
                resolve: {
                    permissions: rolePermissionsResolver
                },
                data: {
                    localizationTitleKey: 'Admission.EventsPage.Header.Title',
                    permissions: [PermissionName.SYSTEM, PermissionName.RULES]
                }
            },
            {
                path: `${AdmissionRouterName.EVENTS}/:eventId`,
                component: AdmissionFeatureDetailsContainer,
                resolve: {
                    permissions: rolePermissionsResolver
                },
                data: {
                    localizationTitleKey: 'Admission.DetailsPage.Header.Title',
                    permissions: [PermissionName.SYSTEM, PermissionName.RULES]
                }
            },
            {
                path: '**',
                redirectTo: AdmissionRouterName.SETTINGS
            }
        ]
    }
];

@NgModule({
    imports: [RouterModule.forChild(routes)],
    exports: [RouterModule]
})
export class AdmissionFeatureRoutingModule {}
