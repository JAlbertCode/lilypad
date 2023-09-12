import { ethers } from 'hardhat'
import {
  BigNumberish,
  AddressLike,
  Signer,
} from 'ethers'
import {
  getWallet,
  fundAccountsWithTokens,
  DEFAULT_TOKEN_SUPPLY,
  DEFAULT_TOKENS_PER_ACCOUNT,
} from '../utils/web3'
import {
  LilypadToken,
  LilypadPayments,
  LilypadStorage,
  LilypadController,
} from '../typechain-types'
import {
  SharedStructs,
} from '../typechain-types/contracts/LilypadStorage'

/*

  DEPLOYMENT

*/
export async function deployContract<T extends any>(
  name: string,
  signer: Signer,
  args: any[] = [],
): Promise<T>{
  const factory = await ethers.getContractFactory(
    name,
    signer,
  )
  const contract = await factory.deploy(...args) as unknown as T
  return contract
}

export async function deployToken(
  signer: Signer,
  tokenSupply: BigNumberish = DEFAULT_TOKEN_SUPPLY,
  testMode = false,
) {
  return deployContract<LilypadToken>(testMode ? 'LilypadTokenTestable' : 'LilypadToken', signer, [
    'LilyPad',
    'LLY',
    tokenSupply,
  ])
}

export async function deployPayments(
  signer: Signer,
  tokenAddress: AddressLike,
  testMode = false,
) {
  const payments = await deployContract<LilypadPayments>(testMode ? 'LilypadPaymentsTestable' : 'LilypadPayments', signer)
  await payments
    .connect(signer)
    .initialize(tokenAddress)
  return payments
}

export async function deployStorage(
  signer: Signer,
  testMode = false,
) {
  return deployContract<LilypadStorage>(testMode ? 'LilypadStorageTestable' : 'LilypadStorage', signer)
}

export async function deployController(
  signer: Signer,
  storageAddress: AddressLike,
  paymentsAddress: AddressLike,
) {
  const controller = await deployContract<LilypadController>('LilypadController', signer)
  await controller
    .connect(signer)
    .initialize(storageAddress, paymentsAddress)
  return controller
}

/*

  FIXTURES

  these are thin wrappers around our web3 utils lib

  used by tests to prepare env for unit tests

*/

/*

  TOKEN

*/

// setup the token in test mode so we can call functions on it directly
// without the ControllerOwnable module kicking in
export async function setupTokenFixture({
  testMode = false,
  withFunds = false,
  controllerAddress,
}: {
  testMode?: boolean,
  withFunds?: boolean,
  controllerAddress?: AddressLike,
}) {
  const admin = getWallet('admin')
  const token = await deployToken(
    admin,
    DEFAULT_TOKEN_SUPPLY,
    testMode,
  )
  if(withFunds) {
    await fundAccountsWithTokens(token, DEFAULT_TOKENS_PER_ACCOUNT)
  }
  if(controllerAddress) {
    await (token as any)
      .connect(admin)
      .setControllerAddress(controllerAddress)
  }
  return token
}

/*

  PAYMENTS

*/

// setup the token in non-test mode but the payments in test mode
// then we can call functions directly on the payments contract
// and the token contract will check with ControllerOwnable
export async function setupPaymentsFixture({
  testMode = false,
  withFunds = false,
  controllerAddress,
}: {
  testMode?: boolean,
  withFunds?: boolean,
  controllerAddress?: AddressLike,
}) {
  const admin = getWallet('admin')
  const token = await setupTokenFixture({
    testMode: false,
    withFunds,
  })
  const payments = await deployPayments(admin, token.getAddress(), testMode)
  await (token as any)
    .connect(admin)
    .setControllerAddress(payments.getAddress())
  if(controllerAddress) {
    await (payments as any)
      .connect(admin)
      .setControllerAddress(controllerAddress)
  }
  return {
    token,
    payments,
  }
}

/*

  TOKEN

*/

// setup the token in test mode so we can call functions on it directly
// without the ControllerOwnable module kicking in
export async function setupStorageFixture({
  testMode = false,
  controllerAddress,
}: {
  testMode?: boolean,
  controllerAddress?: AddressLike,
}) {
  const admin = getWallet('admin')
  const storage = await deployStorage(
    admin,
    testMode,
  )
  if(controllerAddress) {
    await (storage as any)
      .connect(admin)
      .setControllerAddress(controllerAddress)
  }
  return storage
}

export async function setupControllerFixture({
  withFunds = false,
}: {
  withFunds?: boolean,
}) {
  const admin = getWallet('admin')
  const {
    token,
    payments,
  } = await setupPaymentsFixture({
    withFunds,
  })
  const storage = await setupStorageFixture({})
  const storageAddress = await storage.getAddress()
  const paymentsAddress = await payments.getAddress()
  const controller = await deployController(
    admin,
    storageAddress,
    paymentsAddress,
  )
  const controllerAddress = await controller.getAddress()
  await (payments as any)
    .connect(admin)
    .setControllerAddress(controllerAddress)
  await (storage as any)
    .connect(admin)
    .setControllerAddress(controllerAddress)
  return {
    token,
    payments,
    storage,
    controller,
  }
}

export const DEFAUT_TIMEOUT_TIME = 60 * 60
export const DEFAUT_TIMEOUT_COLLATERAL = 1
export const DEFAULT_PRICING_INSTRUCTION_PRICE = 1
export const DEFAULT_PRICING_PAYMENT_COLLATERAL = 1
export const DEFAULT_PRICING_RESULTS_COLLATERAL_MULTIPLE = 1
export const DEFAULT_PRICING_MEDIATION_FEE = 1

export function getDefaultTimeouts() {
  const defaultTimeout: SharedStructs.DealTimeoutStruct = {
    timeout: ethers.getBigInt(DEFAUT_TIMEOUT_TIME),
    collateral: ethers.getBigInt(DEFAUT_TIMEOUT_COLLATERAL),
  }
  const defaultTimeoutNoCost: SharedStructs.DealTimeoutStruct = {
    timeout: ethers.getBigInt(DEFAUT_TIMEOUT_TIME),
    collateral: ethers.getBigInt(0),
  }
  const ret: SharedStructs.DealTimeoutsStruct = {
    agree: defaultTimeoutNoCost,
    submitResults: defaultTimeout,
    judgeResults: defaultTimeout,
    mediateResults: defaultTimeoutNoCost,
  }
  return ret
}

export function getDefaultPricing() {
  const ret: SharedStructs.DealPricingStruct = {
    instructionPrice: ethers.getBigInt(DEFAULT_PRICING_INSTRUCTION_PRICE),
    paymentCollateral: ethers.getBigInt(DEFAULT_PRICING_PAYMENT_COLLATERAL),
    resultsCollateralMultiple: ethers.getBigInt(DEFAULT_PRICING_RESULTS_COLLATERAL_MULTIPLE),
    mediationFee: ethers.getBigInt(DEFAULT_PRICING_MEDIATION_FEE),
  }
  return ret
}