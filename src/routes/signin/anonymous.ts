import { RequestHandler } from 'express';

import { ENV } from '@/utils/env';
import { getSignInResponse } from '@/utils/tokens';
import { insertUser } from '@/utils/user';

type BodyType = {
  locale: string;
  displayName?: string;
  metadata: Record<string, unknown>;
};

export const signInAnonymousHandler: RequestHandler<{}, {}, BodyType> = async (
  req,
  res
) => {
  if (!ENV.AUTH_ANONYMOUS_USERS_ENABLED) {
    return res.boom.notFound('Anonymous users are not enabled');
  }
  const { locale, displayName = 'Anonymous User' } = req.body;

  // restructure user roles to be inserted in GraphQL mutation
  const userRoles = [{ role: 'anonymous' }];

  // insert user
  const user = await insertUser({
    displayName,
    locale,
    roles: {
      data: userRoles,
    },
    defaultRole: 'anonymous',
    isAnonymous: true,
    metadata: {},
  });

  const signInResponse = await getSignInResponse({
    userId: user.id,
    checkMFA: false,
  });

  return res.send(signInResponse);
};
