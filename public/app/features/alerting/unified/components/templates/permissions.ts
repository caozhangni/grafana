import { AccessControlAction } from 'app/types/accessControl';

/**
 * List of all permissions that allow templates read/write functionality
 */

export const PERMISSIONS_TEMPLATES = [
  AccessControlAction.AlertingTemplatesRead,
  AccessControlAction.AlertingTemplatesWrite,
  AccessControlAction.AlertingTemplatesDelete,
];
