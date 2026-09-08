import { type FormEvent, useState } from 'react'
import { LockKeyhole, LogIn } from 'lucide-react'
import { useTranslation } from 'react-i18next'
import { useQueryClient } from '@tanstack/react-query'
import { loginRemoteAccess, remoteAccessQuery } from '@/features/remote-access/api'
import { Button } from '@/components/ui/button'
import { Input } from '@/components/ui/input'
import { Field, FieldDescription, FieldError, FieldGroup, FieldLabel } from '@/components/ui/field'
import { Card, CardContent, CardDescription, CardFooter, CardHeader, CardTitle } from '@/components/ui/card'

export function RemoteAccessLogin({ initialError }: { initialError?: string }) {
  const { t } = useTranslation()
  const queryClient = useQueryClient()
  const [username, setUsername] = useState('')
  const [password, setPassword] = useState('')
  const [submitting, setSubmitting] = useState(false)
  const [error, setError] = useState(initialError ?? '')

  const submit = async (event: FormEvent) => {
    event.preventDefault()
    setSubmitting(true)
    setError('')
    try {
      const status = await loginRemoteAccess(username.trim(), password)
      setPassword('')
      queryClient.setQueryData(remoteAccessQuery.queryKey, status)
    } catch (e) {
      console.warn('[remote-access] Login failed', e)
      setError((e as Error).message || t('remoteAccess.loginFailed'))
    } finally {
      setSubmitting(false)
    }
  }

  return (
    <main data-nova-app-shell="true" className="flex min-h-dvh items-center justify-center bg-background p-4 text-foreground">
      <Card className="w-full max-w-sm">
        <CardHeader>
          <LockKeyhole className="mb-2 size-6" aria-hidden="true" />
          <CardTitle><h1>{t('remoteAccess.title')}</h1></CardTitle>
          <CardDescription>{t('remoteAccess.description')}</CardDescription>
        </CardHeader>
        <CardContent>
          <form id="remote-access-login" onSubmit={submit}>
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="remote-username">{t('remoteAccess.username')}</FieldLabel>
                <Input id="remote-username" name="username" autoFocus value={username} onChange={event => setUsername(event.target.value)} autoComplete="username" required disabled={submitting} />
              </Field>
              <Field>
                <FieldLabel htmlFor="remote-password">{t('remoteAccess.password')}</FieldLabel>
                <Input id="remote-password" name="password" value={password} onChange={event => setPassword(event.target.value)} type="password" autoComplete="current-password" required disabled={submitting} />
                <FieldDescription>{t('remoteAccess.rememberHint')}</FieldDescription>
              </Field>
              {error && <FieldError>{error}</FieldError>}
            </FieldGroup>
          </form>
        </CardContent>
        <CardFooter>
          <Button className="w-full" form="remote-access-login" type="submit" disabled={submitting || !username.trim() || !password}>
            <LogIn data-icon="inline-start" />
            {submitting ? t('remoteAccess.signingIn') : t('remoteAccess.signIn')}
          </Button>
        </CardFooter>
      </Card>
    </main>
  )
}
