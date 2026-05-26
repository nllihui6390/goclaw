#!/usr/bin/env python3
# -*- coding: utf-8 -*-
"""
股票查询工具
支持 A股、港股实时行情查询
使用方法：python stock_query.py --code=600519
         python stock_query.py --code=00700
         python stock_query.py --name=贵州茅台
"""

import requests
import sys
import json
import argparse

# 股票名称到代码的映射（常用股票）
STOCK_NAME_MAP = {
    "茅台": "600519",
    "贵州茅台": "600519",
    "宁德时代": "300750",
    "比亚迪": "002594",
    "平安": "601318",
    "中国平安": "601318",
    "招商银行": "600036",
    "兴业银行": "601166",
    "浦发银行": "600000",
    "工行": "601398",
    "工商银行": "601398",
    "建行": "601939",
    "建设银行": "601939",
    "农行": "601288",
    "中国银行": "601988",
    "中行": "601988",
    "五粮液": "000858",
    "泸州老窖": "000568",
    "山西汾酒": "600809",
    "海天味业": "603259",
    "恒瑞医药": "600276",
    "药明康德": "603259",
    "迈瑞医疗": "300760",
    "爱尔眼科": "300015",
    "腾讯": "00700",
    "腾讯控股": "00700",
    "阿里巴巴": "09988",
    "美团": "03690",
    "小米": "01810",
    "京东": "09618",
    "网易": "09999",
    "百度": "09888",
    "快手": "01024",
    "携程": "09961",
}


def get_market(code: str) -> tuple:
    """判断市场代码"""
    code = code.strip()
    
    # 港股 (以 0/8/9 开头，或 5位数字)
    if len(code) == 5 or code.startswith(('0', '8', '9')):
        return f"116.{code}", get_hk_name(code)
    
    # A股
    if code.startswith(('6', '9')):
        return f"1.{code}", get_a_stock_name(code)
    elif code.startswith(('0', '3')):
        return f"0.{code}", get_a_stock_name(code)
    else:
        # 尝试默认沪市
        return f"1.{code}", get_a_stock_name(code)


def get_a_stock_name(code: str) -> str:
    """获取A股股票名称"""
    try:
        url = "https://push2.eastmoney.com/api/qt/stock/get"
        params = {"secid": f"1.{code}" if code.startswith('6') or code.startswith('9') else f"0.{code}", "fields": "f57"}
        resp = requests.get(url, params=params, timeout=5)
        data = resp.json()
        if data.get("data"):
            return data["data"].get("f57", code)
    except:
        pass
    return code


def get_hk_name(code: str) -> str:
    """获取港股股票名称"""
    try:
        url = "https://push2.eastmoney.com/api/qt/stock/get"
        params = {"secid": f"116.{code}", "fields": "f57"}
        resp = requests.get(url, params=params, timeout=5)
        data = resp.json()
        if data.get("data"):
            return data["data"].get("f57", code)
    except:
        pass
    return code


def resolve_stock(code_or_name: str) -> str:
    """解析股票代码或名称"""
    code_or_name = code_or_name.strip()
    
    # 如果是纯数字，直接返回
    if code_or_name.isdigit():
        return code_or_name
    
    # 如果是名称，查找代码
    if code_or_name in STOCK_NAME_MAP:
        return STOCK_NAME_MAP[code_or_name]
    
    # 模糊匹配
    for name, code in STOCK_NAME_MAP.items():
        if name in code_or_name or code_or_name in name:
            return code
    
    return code_or_name


def query_stock(code: str, name: str = None) -> dict:
    """通过东方财富API查询股票实时行情"""
    
    # 如果提供了名称，先解析为代码
    if name:
        code = resolve_stock(name)
    
    if not code:
        return {"error": "请提供股票代码或名称"}
    
    secid, stock_name = get_market(code)
    
    # 东方财富 API
    url = "https://push2.eastmoney.com/api/qt/stock/get"
    params = {
        "secid": secid,
        "fields": "f43,f44,f45,f46,f47,f48,f49,f50,f51,f52,f57,f58,f84,f85,f128,f140,f141,f115,f117,f127,f128,f162,f163,f164,f167,f168,f169,f170,f171,f173,f177,f187,f188,f189,f190,f191,f192,f193,f194,f197,f198,f199,f200,f201,f202,f203,f204,f205,f206,f207,f208,f209,f210,f211"
    }
    
    try:
        resp = requests.get(url, params=params, timeout=10)
        data = resp.json()
        
        if not data.get("data"):
            return {"error": f"未找到股票 {code} 的数据，请检查代码是否正确"}
        
        d = data["data"]
        
        # 判断市场类型
        is_hk = secid.startswith("116.")
        
        result = {
            "code": code,
            "name": stock_name or d.get("f57", code),
            "market": "港股" if is_hk else "A股",
            "price": d.get("f43", 0) / 1000 if d.get("f43") else 0,  # 最新价
            "change_pct": d.get("f170", 0) / 100 if d.get("f170") else 0,  # 涨跌幅%
            "change": d.get("f171", 0) / 1000 if d.get("f171") else 0,  # 涨跌额
            "high": d.get("f44", 0) / 1000 if d.get("f44") else 0,
            "low": d.get("f45", 0) / 1000 if d.get("f45") else 0,
            "open": d.get("f46", 0) / 1000 if d.get("f46") else 0,
            "prev_close": d.get("f47", 0) / 1000 if d.get("f47") else 0,
            "volume": d.get("f48", 0),  # 成交量(股)
            "amount": d.get("f49", 0),  # 成交额(元)
            "amplitude": d.get("f190", 0) / 100 if d.get("f190") else 0,  # 振幅%
            "turnover": d.get("f168", 0) / 100 if d.get("f168") else 0,  # 换手率%
        }
        
        # A股额外字段
        if not is_hk:
            result["total_mv"] = d.get("f84", 0) / 100000000 if d.get("f84") else 0  # 总市值(亿)
            result["circ_mv"] = d.get("f85", 0) / 100000000 if d.get("f85") else 0  # 流通市值(亿)
            result["pe"] = d.get("f162", 0) / 100 if d.get("f162") else 0  # 市盈率TTM
            result["pb"] = d.get("f167", 0) / 100 if d.get("f167") else 0  # 市净率
            result["52_high"] = d.get("f194", 0) / 1000 if d.get("f194") else 0  # 52周最高
            result["52_low"] = d.get("f193", 0) / 1000 if d.get("f193") else 0  # 52周最低
        
        # 格式化数值
        result["volume_str"] = format_number(result["volume"])
        result["amount_str"] = format_amount(result["amount"])
        if "total_mv" in result:
            result["total_mv_str"] = f"{result['total_mv']:.2f}亿"
        if "circ_mv" in result:
            result["circ_mv_str"] = f"{result['circ_mv']:.2f}亿"
        
        return result
        
    except Exception as e:
        return {"error": f"查询出错: {str(e)}"}


def format_number(num: int) -> str:
    """格式化数字"""
    if num >= 100000000:
        return f"{num/100000000:.2f}亿"
    elif num >= 10000:
        return f"{num/10000:.2f}万"
    return str(num)


def format_amount(amount: int) -> str:
    """格式化成交额"""
    if amount >= 100000000:
        return f"{amount/100000000:.2f}亿元"
    elif amount >= 10000:
        return f"{amount/10000:.2f}万元"
    return str(amount)


def main():
    parser = argparse.ArgumentParser(description="股票查询工具")
    parser.add_argument("--code", "-c", type=str, help="股票代码 (如: 600519, 00700)")
    parser.add_argument("--name", "-n", type=str, help="股票名称 (如: 贵州茅台, 腾讯控股)")
    parser.add_argument("--json", "-j", action="store_true", help="输出JSON格式")
    
    args = parser.parse_args()
    
    if not args.code and not args.name:
        print(json.dumps({"error": "请提供 --code 或 --name 参数"}, ensure_ascii=False, indent=2))
        sys.exit(1)
    
    result = query_stock(args.code, args.name)
    
    if args.json:
        print(json.dumps(result, ensure_ascii=False, indent=2))
    else:
        # 友好格式输出
        if "error" in result:
            print(f"❌ {result['error']}")
            sys.exit(1)
        
        # 涨跌幅颜色
        change_pct = result["change_pct"]
        trend = "📈" if change_pct > 0 else "📉" if change_pct < 0 else "➡️"
        
        print(f"\n{'='*50}")
        print(f"📊 {result['name']} ({result['code']}) - {result['market']}")
        print(f"{'='*50}")
        print(f"💰 当前价格: {result['price']:.2f} 元")
        print(f"{trend} 涨跌幅: {result['change_pct']:+.2f}% ({result['change']:+.2f}元)")
        print(f"📈 最高价: {result['high']:.2f} 元")
        print(f"📉 最低价: {result['low']:.2f} 元")
        print(f"📊 开盘价: {result['open']:.2f} 元")
        print(f"📌 昨收价: {result['prev_close']:.2f} 元")
        print(f"📉 振幅: {result['amplitude']:.2f}%")
        print(f"🔄 换手率: {result['turnover']:.2f}%")
        print(f"📊 成交量: {result['volume_str']}")
        print(f"💵 成交额: {result['amount_str']}")
        
        if result.get("total_mv_str"):
            print(f"🏢 总市值: {result['total_mv_str']}")
        if result.get("circ_mv_str"):
            print(f"🏦 流通值: {result['circ_mv_str']}")
        if result.get("pe"):
            print(f"📊 市盈率TTM: {result['pe']:.2f}")
        if result.get("pb"):
            print(f"📊 市净率: {result['pb']:.2f}")
        
        print(f"{'='*50}\n")


if __name__ == "__main__":
    main()