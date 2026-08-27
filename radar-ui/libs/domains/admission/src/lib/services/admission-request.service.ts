import { Injectable } from '@angular/core';
import { Observable, filter, map, switchMap, take } from 'rxjs';

import { ApiEmptyRequest, ApiService } from '@cs/api';

import {
    AdmissionMonitor,
    AdmissionMonitorConfig,
    CreateAdmissionMonitorRequest,
    EmptyAdmissionResponse
} from '../interfaces';

@Injectable({
    providedIn: 'root'
})
export class AdmissionRequestService {
    constructor(private readonly apiService: ApiService) {}

    getAdmissionMonitor(): Observable<AdmissionMonitor> {
        return this.apiService.get<ApiEmptyRequest, AdmissionMonitor>('config/admission-monitor');
    }

    createAdmissionMonitor(config: AdmissionMonitorConfig): Observable<AdmissionMonitor> {
        return this.apiService
            .post<CreateAdmissionMonitorRequest, EmptyAdmissionResponse>('config/admission-monitor', { config })
            .pipe(
                map((response) => response && !Object.keys(response).length),
                filter((isCreated) => isCreated),
                switchMap(() => this.getAdmissionMonitor().pipe(take(1)))
            );
    }
}
