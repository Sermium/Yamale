/**
 * What an enrolment result means for a key made on this device.
 *
 * The app only ever enrols a brand-new key. A service that answers "already
 * enrolled" (409) is answering for the EMAIL, not for this key: it may hold
 * another key's copies, which cannot rebuild this one. So any 409 means this
 * account is not protected.
 */

export function protectionOutcome(result: { alreadyEnrolled: readonly string[] }): 'enrolled' | 'email-in-use' {
  return result.alreadyEnrolled.length === 0 ? 'enrolled' : 'email-in-use';
}
