export enum AdmissionRouterName {
    SETTINGS = 'settings',
    EVENTS = 'events'
}

export interface AdmissionNavigationTab {
    path: AdmissionRouterName;
    localizationKey: string;
}
