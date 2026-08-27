import { AdmissionNavigationTab, AdmissionRouterName } from '../interfaces/admission-navigation.interface';

export const ADMISSION_NAVIGATION_TABS: AdmissionNavigationTab[] = [
    {
        path: AdmissionRouterName.SETTINGS,
        localizationKey: 'Admission.Pseudo.Navigation.Settings'
    },
    {
        path: AdmissionRouterName.EVENTS,
        localizationKey: 'Admission.Pseudo.Navigation.Events'
    }
];
