import { pwnedPassword } from 'hibp';
import Joi from 'joi';
import { ENV } from '../utils/env';
import { EmailValidator } from './validators';

export const uuidRegex =
  /\b[0-9a-f]{8}\b-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-\b[0-9a-f]{12}\b/;

export const password = Joi.string().example('Str0ngPassw#ord-94|%');

export const passwordInsert = password
  .min(ENV.AUTH_PASSWORD_MIN_LENGTH)
  .description(
    `A password of minimum ${ENV.AUTH_PASSWORD_MIN_LENGTH} characters`
  )
  .external(async (value) => {
    if (ENV.AUTH_PASSWORD_HIBP_ENABLED && (await pwnedPassword(value))) {
      throw new Joi.ValidationError(
        'Password is too weak (it has been pnwed)',
        [],
        value
      );
    } else return value;
  }, `When HIBP is enabled, will check if the password is too weak`);

export const email = Joi.string()
  .email()
  .custom(EmailValidator)
  .example('john.smith@nhost.io')
  .description('A valid email');

export const locale = Joi.string()
  .length(2)
  .example(ENV.AUTH_LOCALE_DEFAULT)
  .default(ENV.AUTH_LOCALE_DEFAULT)
  .description(`A two-characters locale`)
  .valid(...ENV.AUTH_LOCALE_ALLOWED_LOCALES);

export const defaultRole = Joi.string()
  .default(ENV.AUTH_USER_DEFAULT_ROLE)
  .example('user')
  .valid(...ENV.AUTH_USER_DEFAULT_ALLOWED_ROLES);

export const allowedRoles = Joi.array()
  .items(Joi.string())
  .default(ENV.AUTH_USER_DEFAULT_ALLOWED_ROLES)
  .example(['user', 'me'])
  .valid(...ENV.AUTH_USER_DEFAULT_ALLOWED_ROLES);

export const displayName = Joi.string().example('John Smith');

export const metadata = Joi.object().default({}).example({
  firstName: 'John',
  lastName: 'Smith',
});

export const redirectTo = Joi.string()
  .default(ENV.AUTH_CLIENT_URL)
  .regex(new RegExp('^' + ENV.AUTH_CLIENT_URL))
  .valid(...ENV.AUTH_ACCESS_CONTROL_ALLOWED_REDIRECT_URLS)
  .example(`${ENV.AUTH_CLIENT_URL}/catch-redirection`);

export const uuid = Joi.string()
  .regex(uuidRegex)
  .example('2c35b6f3-c4b9-48e3-978a-d4d0f1d42e24')
  .description('A valid UUID');

export const userId = uuid.description('Id of the user');
export const refreshToken = uuid
  .required()
  .description(
    'Refresh token during authentication or when refreshing the JWT'
  );
