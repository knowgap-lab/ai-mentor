import { notifications } from "@mantine/notifications";

export function showCompletionToast(message: string): void {
  notifications.show({
    message,
    withCloseButton: false,
    autoClose: 4000,
  });
}